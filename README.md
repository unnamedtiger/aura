# Aura

*A build server. Lightweight, extensible, good.*

## Building

Install a recent version of [Go](https://go.dev/).
Run `go build .` inside the `controller` and `runner-native` directories to build the executables.

## Demo Setup

Run the controller with the demo flag (`./controller -demo`) to start it with demo data included.
Open up http://localhost:8420/ and click yourself through the web UI.

## Getting Started

Aura consists of two parts

* the **controller** contains the database, provides the web UI and exposes APIs for everything else to interact with the system
* the **runners** are responsible for executing the build jobs

Both parts can run on the same physical machine.

First start up the controller.
It will store its data in the current working directory:

* the database will be stored in the file `aura.db`
* the directory `artifacts` will contain the logs for the build jobs

Open up the web interface at http://localhost:8420/ and click on Runner Status.
Click on New Runner and give your runner a name.
Back on the Runner Status page the new runner will be listed as offline.

In a new working directory (potentially on another machine) create a `config.json` file for the runner.

```json
{
    "name": "buildbox-windows",
    "controller": "http://localhost:8420",
    "tags": ["native,windows"]
}
```

Set the `name` to what you just provided when creating the runner on the controller.
Set `controller` so that the runner can reach the controller using that information.
Set `tags` to one or more tags that build jobs that this runner can handle will have.
Read more about tags [here](docs/tags.md).

Start the runner in this working directory.
It should contact the controller, get no jobs to run, and then output `Sleeping...` and wait for a minute before trying again.
Once you refresh the Runner Status page of the controller you'll see that your new runner is listed with a recent checkin and all tags you provided are also listed as checked-in recently.

In the web interface click on the **A** logo in the top left and on New Project.
Give your project a name and URL slug and create it.
Now you're ready to submit the first job.

Prepare to `POST` some JSON to `/api/submit`.
With [Curl](https://curl.se/) use the following command:

```sh
curl --header "Content-Type: application/json" --request POST --data "@submit.json"  http://localhost:8420/api/submit
```

In this case the content of the file `submit.json` and in any other case the json you want to submit should look as follows:

```json
{
    "project": "test",
    "entityKey": "rev",
    "entityVal": "1",
    "name": "test",
    "cmd": "echo hello world",
    "tag": "native,windows"
}
```

Set `project` to the slug of your project.
The **entity** is where this job will be attached to.
It is made up of a key-value pair.
Set `entityKey` and `entityVal` accordingly.
Read more about entites [here](docs/entities.md).
Set `name` to the name you want your job to have.
Set `cmd` to the command to run inside the runner.
And finally, set `tag` to a tag that you assigned to the runner so that the job will be picked up.

Once the job ran to completion, browse to the entity inside your project to see the results.
