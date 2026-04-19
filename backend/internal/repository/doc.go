/*
Package repository houses the concrete implementations of the data access layer.

This root package serves as an umbrella directory for various storage mechanisms
and database (e.g., CockroachDB, Redis). The sub-packages within this directory
must implement the Repository interfaces defined in the domain layer, handling
the actual data persistence, retrieval, and caching operations without leaking
database-specific details to the usecase layer.
*/
package repository
