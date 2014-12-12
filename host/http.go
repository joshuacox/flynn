package main

import (
	"errors"
	"net"
	"net/http"

	"github.com/flynn/flynn/Godeps/_workspace/src/github.com/julienschmidt/httprouter"
	"github.com/flynn/flynn/host/types"
	"github.com/flynn/flynn/pkg/httphelper"
	"github.com/flynn/flynn/pkg/rpcplus"
	rpc "github.com/flynn/flynn/pkg/rpcplus/comborpc"
	"github.com/flynn/flynn/pkg/shutdown"
)

func serveHTTP(host *Host, attach *attachHandler, sh *shutdown.Handler) error {
	if err := rpc.Register(host); err != nil {
		return err
	}
	rpc.HandleHTTP()
	http.Handle("/attach", attach)

	l, err := net.Listen("tcp", ":1113")
	if err != nil {
		return err
	}
	sh.BeforeExit(func() { l.Close() })
	go http.Serve(l, nil)
	return nil
}

type Host struct {
	state   *State
	backend Backend
}

func (h *Host) ListJobs(res *map[string]host.ActiveJob) error {
	*res = h.state.Get()
	return nil
}

func (h *Host) GetJob(id string, res *host.ActiveJob) error {
	job := h.state.GetJob(id)
	if job != nil {
		*res = *job
	}
	return nil
}

func (h *Host) StopJob(id string) error {
	job := h.state.GetJob(id)
	if job == nil {
		return errors.New("host: unknown job")
	}
	switch job.Status {
	case host.StatusStarting:
		h.state.SetForceStop(id)
		return nil
	case host.StatusRunning:
		return h.backend.Stop(id)
	default:
		return errors.New("host: job is already stopped")
	}
}

func (h *Host) StreamEvents(id string, stream rpcplus.Stream) error {
	ch := h.state.AddListener(id)
	defer h.state.RemoveListener(id, ch)
	for {
		select {
		case event := <-ch:
			select {
			case stream.Send <- event:
			case <-stream.Error:
				return nil
			}
		case <-stream.Error:
			return nil
		}
	}
}

type HostHandle func(*Host, http.ResponseWriter, *http.Request, httprouter.Params)

// Helper function for wrapping a ClusterHandle into a httprouter.Handles
func hostMiddleware(host *Host, handle HostHandle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		handle(host, w, r, ps)
	}
}

func listJobs(h *Host, w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	// Optionally -- Accept: text/event-stream
	rh := httphelper.NewReponseHelper(w)
	var res *map[string]host.ActiveJob
	err := rh.ListJobs(res)
	if err != nil {
		rh.Error(err)
		return
	}
	rh.JSON(200, res)
}

func getJob(h *Host, w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	rh := httphelper.NewReponseHelper(w)
	id := ps.ByName("id")
	var res *host.ActiveJob
	err := h.GetJob(id, &res)
	if err != nil {
		rh.Error(err)
		return
	}
	rh.JSON(200, res)
}

func stopJob(h *Host, w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	rh := httphelper.NewReponseHelper(w)
	id := ps.ByName("id")
	err := h.StopJob(id)
	if err != nil {
		rh.Error(err)
		return
	}
	rh.WriteHeader(200)
}

func newRouter(host *Host) {
	r := httprouter.New()

	r.GET("/host/jobs", hostMiddleware(host, listJobs))
	r.GET("/host/jobs/:id", hostMiddleware(host, getJob))
	r.DELETE("/host/jobs/:id", hostMiddleware(host, stopJob))
}
