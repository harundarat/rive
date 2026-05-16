/*
Package database provides the low-level database initialization utilities
for the infrastruture layer.

This package is responsible for preparing database connection objects, such
as building DSNs, opening connectionsn, verifying connectivity, and configuring
connection pool settings. The objects create here are intende to be injected
into repository implementations.

This package must not contain business rules or data access/query logic.
Those concerns belong to the usecase and repository layers respectively.
*/
package database
