/*
Package response provides the standard HTTP response envelope and helper
functions for constructing consistent API responses across the application.

All HTTP handlers must use this package to ensure every response follows
a uniform structure, containing a success flag, a data payload, an error
object, and optional pagination metadata.
*/
package response
