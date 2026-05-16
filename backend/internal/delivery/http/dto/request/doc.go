/*
Package request defines structs that represent inbound HTTP request payloads.

Each struct maps to the JSON body, query parameters, or path variables of a
specific endpoint. Validation tags are used to enforce constraints before the
payload is passed to the Usecase layer.
*/
package request
