package sampi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/flynn/flynn/Godeps/_workspace/src/github.com/julienschmidt/httprouter"
	"github.com/flynn/flynn/host/types"
	"github.com/flynn/flynn/pkg/sse"
)

// Cluster
type Cluster struct {
	state *State
}

func NewCluster(state *State) *Cluster {
	return &Cluster{state}
}

// Scheduler Methods

func (s *Cluster) ListHosts(ret *map[string]host.Host) error {
	*ret = s.state.Get()
	return nil
}

func (s *Cluster) AddJobs(req *host.AddJobsReq, res *host.AddJobsRes) error {
	s.state.Begin()
	*res = host.AddJobsRes{}
	for host, jobs := range req.HostJobs {
		if err := s.state.AddJobs(host, jobs); err != nil {
			s.state.Rollback()
			return err
		}
	}
	res.State = s.state.Commit()

	for host, jobs := range req.HostJobs {
		for _, job := range jobs {
			s.state.SendJob(host, job)
		}
	}

	return nil
}

// Host Service methods

func (s *Cluster) RegisterHost(hostID *string, h *host.Host) error {
	*hostID = h.ID
	id := *hostID
	if id == "" {
		return errors.New("sampi: host id must not be blank")
	}

	s.state.Begin()

	if s.state.HostExists(id) {
		s.state.Rollback()
		return errors.New("sampi: host exists")
	}

	jobs := make(chan *host.Job)
	s.state.AddHost(h, jobs)
	s.state.Commit()
	go s.state.sendEvent(id, "add")

	var err error
	// (IceDragon) TODO: send this to code hell, and replace with something better?
outer:
	for {
		select {
		case job := <-jobs:
			// make sure we don't deadlock if there is an error while we're sending
			select {
			case stream.Send <- job:
			case err = <-stream.Error:
				break outer
			}
		case err = <-stream.Error:
			break outer
		}
	}

	s.state.Begin()
	s.state.RemoveHost(id)
	s.state.Commit()
	go s.state.sendEvent(id, "remove")
	if err == io.EOF {
		err = nil
	}
	return err
}

func (s *Cluster) RemoveJobs(hostID string, jobIDs []string) error {
	s.state.Begin()
	s.state.RemoveJobs(hostID, jobIDs...)
	s.state.Commit()
	return nil
}

func (s *Cluster) StreamHostEvents(ch chan host.HostEvent) error {
	s.state.AddListener(ch)
	// (IceDragon) TODO: Figure out how to stream events out and remove listener here
	// without exposing this method to the HTTP interface
	go func() {
		// (IceDragon) TODO: Move this to the HTTP caller
		//<-w.(http.CloseNotifier).CloseNotify()
		go func() {
			// drain to prevent deadlock while removing the listener
			for range ch {
			}
		}()
		s.state.RemoveListener(ch)
		close(ch)
	}()
	return nil
}

// HTTP Route Handles

// TEMP: Helper function for writing out errors in JSON
func httpWriteError(w http.ResponseWriter, e error) {
	enc := json.NewEncoder(w)
	w.WriteHeader(400)
	enc.Encode(e)
}

func listHosts(c *Cluster, w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	// TODO wip
	ret := make(map[string]host.Host)
	err := c.ListHosts(&ret)
	if err != nil {
		httpWriteError(w, err)
		return
	}
	w.WriteHeader(200)
	json.NewEncoder(sse.NewSSEWriter(w)).Encode(ret)
}

func registerHost(c *Cluster, w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	// TODO wip
	enc := json.NewEncoder(w)
	hostID := ps.ByName("id")
	// (IceDragon) TODO: No, just no, or maybe?. I'm not sure, I need to ask if this is the right way
	h := &host.Host{}

	err := c.RegisterHost(hostID, h)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		httpWriteError(w, err)
		return
	}
	w.WriteHeader(200)
}

func addJobs(c *Cluster, w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	// TODO wip
	enc := json.NewEncoder(w)
	res := host.AddJobsRes{}
	req := host.AddJobsReq{}
	err := c.AddJobs(&req, &res)
	if err != nil {
		httpWriteError(w, err)
		return
	}
	w.WriteHeader(200)
	json.NewEncoder(sse.NewSSEWriter(w)).Encode(res)
}

func removeJob(c *Cluster, w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	// TODO wip
	jobIDs := []string{ps.ByName("id")}
	err := c.RemoveJobs(hostID, jobIDs)
	if err != nil {
		httpWriteError(w, err)
		return
	}
	w.WriteHeader(200)
}

func streamHostEvents(c *Cluster, w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	// TODO wip
	ch := make(chan host.HostEvent)
	wr := sse.NewSSEWriter(w)
	enc := json.NewEncoder(wr)
	err := c.StreamHostEvents(ch)
	if err != nil {
		httpWriteError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.WriteHeader(200)
	wr.Flush()
	for data := range ch {
		enc.Encode(data)
		wr.Flush()
	}
}

type ClusterHandle func(*Cluster, http.ResponseWriter, *http.Request, httprouter.Params)

// Helper function for wrapping a ClusterHandle into a httprouter.Handles
func clusterMiddleware(cluster *Cluster, handle ClusterHandle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		handle(cluster, w, r, ps)
	}
}

func NewHTTPClusterRouter(cluster *Cluster) *httprouter.Router {
	r := httprouter.New()
	r.GET("/cluster/hosts", clusterMiddleware(cluster, listHosts))
	r.PUT("/cluster/hosts/:id", clusterMiddleware(cluster, registerHost))
	r.POST("/cluster/hosts/:host_id/jobs", clusterMiddleware(cluster, addJobs))
	r.DELETE("/cluster/hosts/:host_id/jobs/:job_id", clusterMiddleware(cluster, removeJob))
	r.GET("/cluster/events", clusterMiddleware(cluster, streamHostEvents))
	return r
}

// Issues to resolve:
/*
// Since RPC has been removed, these stream(s) need to be replaced with regular channels or something
/home/vagrant/go/src/github.com/flynn/flynn/host/sampi/http.go:78: undefined: stream
/home/vagrant/go/src/github.com/flynn/flynn/host/sampi/http.go:79: undefined: stream
/home/vagrant/go/src/github.com/flynn/flynn/host/sampi/http.go:82: undefined: stream
/home/vagrant/go/src/github.com/flynn/flynn/host/sampi/http.go:140: undefined: stream

// still wondering why this is throwing a hissy fit
/home/vagrant/go/src/github.com/flynn/flynn/host/sampi/http.go:140: undefined: h
/home/vagrant/go/src/github.com/flynn/flynn/host/sampi/http.go:140: too many arguments in call to c.RegisterHost
/home/vagrant/go/src/github.com/flynn/flynn/host/sampi/http.go:164: undefined: hostID
*/
