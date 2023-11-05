# Changelog

## 0.3.0 - 2023-11-05

These are **BREAKING CHANGES** to the API endpoints.
Update controller and runners at the same time.
Adjust external applications if necessary.

* Moved API data types out to new api module
* Made API responses consistent
* Added field to job submission API to insert entity into collection(s)
* Added list of environment variables to job page
* Updated styles

## 0.2.0 - 2023-10-12

These are **BREAKING CHANGES** to the database schema.
Recreate your database, there is no migration available.

* Added authentication to all API endpoints
* Added `auth` columns to several database tables

## 0.1.0 - 2023-10-01

* Initial release
