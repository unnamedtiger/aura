# Authorization / Authentication in Aura

Aura has four types of keys with different scopes.

The **admin key** is mainly used to create new projects and runners.
It can also be used in every API endpoint.
There is only ever one admin key; it is printed out to the console on startup.

A **project key** is assigned to a single project and used to submit jobs for that project.
It gets generated when a project is created in the web UI.

A **runner key** is configured in a runner and secures communication between runner and controller.
It gets generated when a runner is created in the web UI.

A **job key** is valid while that job is executed in a runner.
It is created when a job is sent to a runner and invalidated once the runner marks the job as completed.
