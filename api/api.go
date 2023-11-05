// Package api contains type definitions and utility functions for interacting with the Aura API.
//
// # Usage
//
//  1. Create an AuraApi
//  2. Use its methods to build an endpoint URL
//  3. POST the specified request encoded in JSON to that endpoint.
//     Set the "Content-Type" header to "application/json".
//     Set the authentication if necessary.
//  4. Parse the JSON returned as a response into the specified response or Status depending on status code.
//
// # Example
//
//	requestData := XxxRequest{}
//	requestPayload, _ := json.Marshal(requestData)
//	request, _ := http.NewRequest(http.MethodPost, auraApi.Xxx(), bytes.NewBuffer(requestPayload))
//	request.Header.Set("Content-Type", "application/json")
//	request.Header.Set("Authorization", "Bearer "+myKey)
//	response, _ := http.DefaultClient.Do(request)
//	var responseData XxxResponse
//	_ = json.NewDecoder(response.Body).Decode(&responseData)
//
// # Authentication
//
// Aura has four types of keys with different scopes:
//
//   - The ADMINKEY is mainly used to create new projects and runners but it can be used in every API endpoint.
//     There is only ever one admin key; it is printed out to the console on first startup.
//   - A PROJECTKEY is assigned to a single project and used to submit jobs for that project.
//     It gets generated when a project is created in the web UI.
//   - A RUNNERKEY is configured in a runner and secures communication between runner and controller.
//     It gets generated when a runner is created in the web UI.
//   - A JOBKEY is valid while that job is executed in a runner.
//     It is created when a job is sent to a runner and invalidated once the runner marks the job as completed.
//
// If an endpoint requires authentication, the method will contain the valid key types.
// Provide the key as a bearer token in the "Authorization" header.
//
//	request.Header.Set("Authorization", "Bearer "+myKey)
package api

import (
	"fmt"
	"net/url"
	"regexp"
)

// Anything that is a slug must match this regular expression
var SlugRegex = regexp.MustCompile(`^[0-9A-Za-z-_:\.]{1,260}$`)

// AuraApi provides methods to build URLs for the various API endpoints
type AuraApi struct {
	baseUrl string
}

// New returns a new AuraApi configured with the scheme and host from the given URL
func New(baseUrl *url.URL) *AuraApi {
	return &AuraApi{fmt.Sprintf("%s://%s", baseUrl.Scheme, baseUrl.Host)}
}

// Status is returned with a 4xx or 5xx status code from any endpoint
type Status struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// =============================================================================
// /api/job
// =============================================================================

// As a runner, use this endpoint to mark a job as completed.
//
// Endpoint: /api/job | Auth: AURA_RUNNERKEY
func (a AuraApi) Job() string {
	return fmt.Sprintf("%s/api/job", a.baseUrl)
}

type JobRequest struct {
	// The name of the runner sending the request
	Name string `json:"name"`

	// The id of the job that was just completed
	Id int64 `json:"id"`

	// Exit code of that job
	ExitCode int64 `json:"exitCode"`
}

type JobResponse struct {
	// This struct has been intentionally left empty
}

// =============================================================================
// /api/runner
// =============================================================================

// As a runner, use this endpoint to request new jobs from the controller or simply check in.
//
// Endpoint: /api/runner | Auth: AURA_RUNNERKEY
func (a AuraApi) Runner() string {
	return fmt.Sprintf("%s/api/runner", a.baseUrl)
}

type RunnerRequest struct {
	// The name of the runner sending the request
	Name string `json:"name"`

	// A list of tags this runner requests jobs for, sorted by priority
	Tags []string `json:"tags"`

	// The number of jobs to return.
	// Set to 0 to check in to the controller but not request any new jobs.
	Limit int `json:"limit"`
}

type RunnerResponse struct {
	Jobs []RunnerResponseJob `json:"jobs"`
}

type RunnerResponseJob struct {
	Id        int64  `json:"id"`
	Project   string `json:"project"`
	EntityKey string `json:"entityKey"`
	EntityVal string `json:"entityVal"`
	Name      string `json:"name"`
	JobKey    string `json:"jobKey"`
	Cmd       string `json:"cmd"`
	Env       string `json:"env"`
	Tag       string `json:"tag"`
}

// =============================================================================
// /api/storage
// =============================================================================

// As a job or as a runner, upload artifacts to the controller.
//
// The request body is the content of the file to upload.
// Path may only be "log".
//
// Endpoint: /api/storage/$jobId/$path | Auth: AURA_RUNNERKEY, AURA_JOBKEY
func (a AuraApi) Storage(jobId int64, path string) string {
	return fmt.Sprintf("%s/api/storage/%d/%s", a.baseUrl, jobId, path)
}

type StorageResponse struct {
	// This struct has been intentionally left empty
}

// =============================================================================
// /api/submit
// =============================================================================

// Submit a new job to the controller.
//
// Endpoint: /api/submit | Auth: AURA_PROJECTKEY, AURA_JOBKEY
func (a AuraApi) Submit() string {
	return fmt.Sprintf("%s/api/submit", a.baseUrl)
}

type SubmitRequest struct {
	// Id of the job that submits this child-job.
	// Set if and only if you use a JOBKEY.
	ParentJob *int64 `json:"parentJob"`

	// Slug of the project to attach the job to
	Project string `json:"project"`

	// The entity to attach this job to.
	// Will be created if it didn't exist before this.
	// Both parts must match SlugRegex.
	EntityKey string `json:"entityKey"`
	EntityVal string `json:"entityVal"`

	// Name of the new job.
	// Must match SlugRegex.
	Name string `json:"name"`

	// The command to run for the job.
	// Depending on runner may or may not have a shell environment available.
	Cmd string `json:"cmd"`

	// The content of a .env file to include when running the job.
	// List of key-value pair separated with '\n'.
	// Key separated from value by first '=' in line.
	Env string `json:"env"`

	// The tag used to find a usable runner
	Tag string `json:"tag"`

	// Map of key to value for collections to include this entity in.
	// May only be used if you use a PROJECTKEY.
	Collections map[string]string `json:"collections"`

	// List of job ids of jobs that need to succeed for this job to start.
	// If a preceding job fails, this job gets cancelled and not executed.
	PrecedingJobs []int64 `json:"precedingJobs"`

	// Unix timestamp (in seconds) of the earliest possibly start for this job.
	// Leave out (nil) to not restrict.
	EarliestStart *int64 `json:"earliestStart"`
}

type SubmitResponse struct {
	// The id of the newly created job
	Id int64 `json:"id"`
}
