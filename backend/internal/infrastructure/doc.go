/*
Package infrastructure handles the initialization of connections to external services.

This includes creating connection pools for PostgreSQL, initializing Redis clients,
or setting up publishers/consumers for a message broker. It provides raw connection
objects that are subsequently injected into the repository layer.
*/
package infrastructure
