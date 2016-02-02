package main

import (
	"encoding/xml"
	"fmt"
	"net/http"

	"github.com/foomo/soap"
)

// FooRequest a simple request
type FooRequest struct {
	XMLName xml.Name `xml:"fooRequest"`
	Foo     string
}

// FooResponse a simple response
type FooResponse struct {
	Bar string
}

// RunServer run a little demo server
func RunServer() {
	soapServer := soap.NewServer()
	soapServer.HandleOperation(
		// SOAPAction
		"operationFoo",
		// tagname of soap body content
		"fooRequest",
		// RequestFactoryFunc - give the server sth. to unmarshal the request into
		func() interface{} {
			return &FooRequest{}
		},
		// OperationHandlerFunc - do something
		func(request interface{}, w http.ResponseWriter, httpRequest *http.Request) (response interface{}, err error) {
			fooRequest := request.(*FooRequest)
			fooResponse := &FooResponse{
				Bar: "Hello \"" + fooRequest.Foo + "\"",
			}
			response = fooResponse
			return
		},
	)
	err := soapServer.ListenAndServe(":8080")
	fmt.Println("exiting with error", err)
}

func main() {
	// see what is going on
	soap.Verbose = true
	RunServer()
}
