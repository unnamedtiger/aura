# Entities

Jobs in Aura are attached to a single entity.
Every entity is part of a single project.
An entity is a key-value pair.
Aura expects a project to have few entity keys but every entity key to have many entity values.

What is a suitable entity depends entirely on your project and where your jobs come from.
You might want to use a unique identifier in your version control system as an entity.

## Examples

* with a distributed version control system
    * `commit/f2ac9babde8397a246240237ea004c7f19a9789f`
* with a centralized version control system
    * `rev/1`
* date-based
    * `nightly/2023-09-23`
* ...
