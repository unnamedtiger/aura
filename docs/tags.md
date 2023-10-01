# Tags

Tags are used in Aura to pick the correct runner for a given build job.
A tag is a string that can contain whatever data you need to include in it.
A build job contains exactly one tag and a runner will generally ask for jobs from a list of tags it can fulfill.

Even though a tag is a single string, it can contain multiple things. You might want to consider the following when setting up your tags:

* the type of the runner
* the OS the runner is on
* the version or flavor of OS
* how much system resources the runner can provide
* special hardware a job could access
* special software or licensed software provided by the runner
* ...

## Examples

* By runner type and OS
    * `native,windows`
    * `docker,linux`
    * ...
