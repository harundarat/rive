/*
Package dto contains Data Transfer Objects used by the HTTP delivery layer.

It is divided into two sub-packages: request for inbound payloads parsed from
HTTP requests, and response for outbound payloads serialized into HTTP responses.
These types are intentionally separated from domain entities to decouple the
transport format from the core business model.
*/
package dto
