/*
Package http is the presentation (Delivery) layer based on HTTP/REST.

This package contains HTTP handlers and routing logic. Its main tasks are
parsing HTTP requests (JSON body, query parameters), invoking the appropriate
Usecase functions, and wrapping the results or errors into a standard
HTTP response format with the correct status codes.
*/
package http
