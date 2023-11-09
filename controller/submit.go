package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/unnamedtiger/aura/api"
)

type Config struct {
}

// NOTE: ProjectName from api.SubmitRequest is ignored, ProjectId is used instead
type Submission struct {
	api.SubmitRequest
	ProjectId int64
}

type SubmitError struct {
	code int
	msg  string
	err  error
}

type SubmitEndpoint interface {
	HandleRequest(r *http.Request) (Submission, *SubmitError)
}

var submitEndpoints map[string]SubmitEndpoint

func InitializeSubmitEndpoints() {
	var cfg Config
	submitEndpoints = map[string]SubmitEndpoint{}
	missingEndpoints := []string{}

	_, err := os.Stat("config.json")
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Fatalln(err)
		}
	} else {
		data, err := os.ReadFile("config.json")
		if err != nil {
			log.Fatalln(err)
		}
		err = json.Unmarshal(data, &cfg)
		if err != nil {
			log.Fatalln(err)
		}
	}

	submitEndpoints[""] = SubmitEndpointGeneric{}
	log.Printf("Initialized generic submit endpoint at /api/submit\n")

	if len(missingEndpoints) > 0 {
		log.Printf("Did not initialize submit endpoint for %s (missing config)\n", strings.Join(missingEndpoints, ", "))
	}
}

func Submit(sub Submission) (int64, *SubmitError) {
	t := time.Now()
	if !slugRegex.MatchString(sub.Name) {
		return 0, &SubmitError{http.StatusBadRequest, "invalid name", nil}
	}

	entity, err := FindEntity(sub.ProjectId, sub.EntityKey, sub.EntityVal)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if !slugRegex.MatchString(sub.EntityKey) {
				return 0, &SubmitError{http.StatusBadRequest, "invalid entityKey", nil}
			}
			if !slugRegex.MatchString(sub.EntityVal) {
				return 0, &SubmitError{http.StatusBadRequest, "invalid entityVal", nil}
			}
			err = CreateEntity(sub.ProjectId, sub.EntityKey, sub.EntityVal, t)
			if err != nil {
				return 0, &SubmitError{http.StatusInternalServerError, "internal server error", err}
			}
			entity, err = FindEntity(sub.ProjectId, sub.EntityKey, sub.EntityVal)
			if err != nil {
				return 0, &SubmitError{http.StatusInternalServerError, "internal server error", err}
			}
		} else {
			return 0, &SubmitError{http.StatusInternalServerError, "internal server error", err}
		}
	}

	earliestStart := t
	if sub.EarliestStart != nil {
		earliestStart = time.Unix(*sub.EarliestStart, 0)
	}
	jobId, err := CreateJob(entity.Id, sub.Name, t, earliestStart, sub.Cmd, sub.Env, sub.Tag)
	if err != nil {
		return 0, &SubmitError{http.StatusInternalServerError, "internal server error", err}
	}

	go submitPostprocessing(sub, jobId, t, entity.Id)
	return jobId, nil
}

func submitPostprocessing(sub Submission, jobId int64, t time.Time, entityId int64) {
	for key, value := range sub.Collections {
		coll, err := FindCollection(sub.ProjectId, key, value)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				err = CreateCollection(sub.ProjectId, key, value, t)
				if err != nil {
					log.Println(err)
					return
				}
			} else {
				log.Println(err)
				return
			}
			coll, err = FindCollection(sub.ProjectId, key, value)
			if err != nil {
				log.Println(err)
				return
			}
		}
		err = InsertEntityIntoCollection(coll.Id, entityId)
		if err != nil {
			log.Println(err)
			return
		}
	}

	for _, precedingJob := range sub.PrecedingJobs {
		_, err := LoadJob(precedingJob)
		if err != nil {
			if errors.Is(err, ErrNotFound) { // ignore invalid preceding jobs
				continue
			}
			log.Println(err)
			return
		}
		err = CreatePrecedingJob(precedingJob, jobId)
		if err != nil {
			log.Println(err)
			return
		}
		// NOTE: loading this job again to avoid race condition
		precedingJob, err := LoadJob(precedingJob)
		if err != nil {
			log.Println(err)
			return
		}
		if precedingJob.Status == StatusCancelled || precedingJob.Status == StatusFailed {
			err = MarkPrecedingJobCompleted(precedingJob.Id)
			if err != nil {
				log.Println(err)
				return
			}
			err = MarkJobDone(jobId, StatusCancelled, 0, t)
			if err != nil {
				log.Println(err)
				return
			}
		} else if precedingJob.Status == StatusSucceeded {
			err = MarkPrecedingJobCompleted(precedingJob.Id)
			if err != nil {
				log.Println(err)
				return
			}
		}
	}

	err := MarkJobCreated(jobId)
	if err != nil {
		log.Println(err)
		return
	}
}

func RouteApiSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/api/submit")
	p = strings.TrimPrefix(p, "/")
	endpoint, found := submitEndpoints[p]
	if !found {
		respondError(w, http.StatusNotFound, "invalid submission endpoint")
		return
	}
	sub, err := endpoint.HandleRequest(r)
	if err != nil {
		log.Println(err.err)
		respondError(w, err.code, err.msg)
		return
	}
	jobId, err := Submit(sub)
	if err != nil {
		log.Println(err.err)
		respondError(w, err.code, err.msg)
		return
	}
	respond(w, http.StatusOK, api.SubmitResponse{Id: jobId})
}

// =============================================================================
// /api/submit
// =============================================================================

type SubmitEndpointGeneric struct {
}

func (SubmitEndpointGeneric) HandleRequest(r *http.Request) (Submission, *SubmitError) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return Submission{}, &SubmitError{http.StatusInternalServerError, "internal server error", err}
	}
	var req api.SubmitRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		return Submission{}, &SubmitError{http.StatusBadRequest, "unable to unmarshal json object", err}
	}
	project, err := FindProjectBySlug(req.Project)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Submission{}, &SubmitError{http.StatusBadRequest, "unknown project", err}
		}
		return Submission{}, &SubmitError{http.StatusInternalServerError, "internal server error", err}
	}

	authHeader := r.Header.Get("Authorization")
	if len(authHeader) == 0 {
		return Submission{}, &SubmitError{http.StatusUnauthorized, "missing authorization header", nil}
	}
	authHeader = strings.TrimPrefix(authHeader, "Bearer ")
	authOk := false
	if strings.HasPrefix(authHeader, PrefixJob) && req.ParentJob != nil {
		parentJob, err := LoadJob(*req.ParentJob)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return Submission{}, &SubmitError{http.StatusBadRequest, "unknown parent job", err}
			}
			return Submission{}, &SubmitError{http.StatusInternalServerError, "internal server error", err}
		}
		parentEntity, err := LoadEntity(parentJob.EntityId)
		if err != nil {
			return Submission{}, &SubmitError{http.StatusInternalServerError, "internal server error", err}
		}
		if project.Id != parentEntity.ProjectId {
			return Submission{}, &SubmitError{http.StatusUnauthorized, "project does not match parent job project", nil}
		}
		if req.EntityKey != parentEntity.Key {
			return Submission{}, &SubmitError{http.StatusUnauthorized, "entityKey does not match parent job entityKey", nil}
		}
		if req.EntityVal != parentEntity.Val {
			return Submission{}, &SubmitError{http.StatusUnauthorized, "entityVal does not match parent job entityVal", nil}
		}
		if len(req.Collections) > 0 {
			return Submission{}, &SubmitError{http.StatusUnauthorized, "may not set collections when using JOBKEY", nil}
		}

		authOk, err = checkJobAuth(parentJob.Auth, authHeader)
		if err != nil {
			return Submission{}, &SubmitError{http.StatusInternalServerError, "internal server error", err}
		}
	} else {
		for key, value := range req.Collections {
			if !slugRegex.MatchString(key) {
				return Submission{}, &SubmitError{http.StatusBadRequest, "invalid collection key", nil}
			}
			if !slugRegex.MatchString(value) {
				return Submission{}, &SubmitError{http.StatusBadRequest, "invalid collection value", nil}
			}
		}

		authOk, err = checkProjectAuth(project.Auth, authHeader)
		if err != nil {
			return Submission{}, &SubmitError{http.StatusInternalServerError, "internal server error", err}
		}
	}
	if !authOk {
		return Submission{}, &SubmitError{http.StatusUnauthorized, "unauthorized", nil}
	}
	return Submission{req, project.Id}, nil
}
