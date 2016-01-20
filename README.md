# SOAP is dead - long live SOAP

First of all do not write SOAP services if you can avoid it! It is over.

If you can not avoid it this package might help.

```go
package main

import "github.com/foomo/soap"

type FooRequest struct {
	Foo string
}

type FooResponse struct {
	Bar string
}

func RunServer() {
	soapServer := soap.NewServer("127.0.0.1:8080")

	soapServer.HandleOperation(
		"operationFoo",
		func() interface{} {
			return &FooRequest{}
		},
		func(request interface{}) (response interface{}, err error) {
			fooRequest := request.(FooRequest)
			fooResponse := &FooResponse{
				Bar: "Hello " + fooRequest.Foo,
			}
			response = fooResponse
			return
		},
	)

}

func main() {
	RunServer()
}

```
