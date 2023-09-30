package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/google/shlex"
)

type Config struct {
	Name       string   `json:"name"`
	Controller string   `json:"controller"`
	Tags       []string `json:"tags"`
}

type Request struct {
	Name  string   `json:"name"`
	Tags  []string `json:"tags"`
	Limit int      `json:"limit"`
}

type Job struct {
	Id  int64
	Cmd string
}

type Results struct {
	Name     string `json:"name"`
	Id       int64  `json:"id"`
	ExitCode int64  `json:"exitCode"`
}

func main() {
	data, err := os.ReadFile("config.json")
	if err != nil {
		log.Fatalln(err)
	}
	var cfg Config
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		log.Fatalln(err)
	}
	if len(cfg.Name) == 0 {
		log.Fatalln("invalid name")
	}
	if len(cfg.Controller) == 0 {
		log.Fatalln("invalid controller")
	}
	if len(cfg.Tags) == 0 {
		log.Fatalln("invalid tags")
	}
	log.Printf("Starting runner %s...", cfg.Name)

	for {
		req := Request{Name: cfg.Name, Tags: cfg.Tags, Limit: 1}
		reqData, err := json.Marshal(req)
		if err != nil {
			log.Fatalln(err)
		}
		respObj, err := http.Post(cfg.Controller+"/api/runner", "application/json", bytes.NewBuffer(reqData))
		if err != nil {
			log.Fatalln(err)
		}
		var resp []Job
		err = json.NewDecoder(respObj.Body).Decode(&resp)
		if err != nil {
			log.Fatalln(err)
		}
		respObj.Body.Close()

		if len(resp) > 0 {
			for _, job := range resp {
				log.Printf("Running job %d...", job.Id)
				runJob(cfg, job)
			}
		} else {
			log.Println("Sleeping...")
			time.Sleep(time.Minute)
		}
	}
}

func runJob(cfg Config, job Job) {
	exitCode := 0
	out := []byte{}
	parts, err := shlex.Split(job.Cmd)
	if err != nil {
		log.Fatalln(err)
		exitCode = -1
	} else {
		partsArgs := []string{}
		if len(parts) > 1 {
			partsArgs = parts[1:]
		}
		cmd := exec.Command(parts[0], partsArgs...)
		out, err = cmd.CombinedOutput()
		if err != nil {
			exitError, ok := err.(*exec.ExitError)
			if ok {
				exitCode = exitError.ExitCode()
			} else {
				log.Fatalln(err)
				exitCode = -1
			}
		}
		log.Println(string(out))
	}

	req := Results{Name: cfg.Name, Id: job.Id, ExitCode: int64(exitCode)}
	reqData, err := json.Marshal(req)
	if err != nil {
		log.Fatalln(err)
	}
	_, err = http.Post(cfg.Controller+"/api/job", "application/json", bytes.NewBuffer(reqData))
	if err != nil {
		log.Fatalln(err)
	}
}
