package main

import (
	"errors"
	"net"
	"net/http"

	"github.com/flynn/flynn/Godeps/_workspace/src/github.com/julienschmidt/httprouter"
)

func ListHosts(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {

}

func newRouter() {
	r := httprouter.New()
	/// Sampi
	// hosts
	r.GET("/cluster/hosts", ListHosts)
	r.PUT("/cluster/hosts/:id", RegisterHost)
	// jobs
	r.POST("/cluster/jobs", AddJobs)
	r.DLEETE("/cluster/jobs/:id", RemoveJob)
	// events -- Accept: text/event-stream
	r.GET("/cluster/events", StreamHostEvents)

	/// Host
	// || -- Accept: text/event-stream
	r.GET("/host/jobs", ListJobs)
	r.GET("/host/jobs/:id", GetJob)
	r.DELETE("/host/jobs/:id", StopJob)
}

/*
# Sampi

## ListHosts

GET /cluster/hosts


## AddJobs

POST /cluster/jobs


## RegisterHost

PUT /cluster/hosts/:id


## RemoveJob

DELETE /cluster/jobs/:id


## StreamHostEvents

GET /cluster/events
Accept: text/event-stream


# Host

## ListJobs

GET /host/jobs


## GetJob

GET /host/jobs/:id


## StopJob

DELETE /host/jobs/:id


## StreamEvents

GET /host/jobs
Accept: text/event-stream

*/
