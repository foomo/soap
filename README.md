# SOAP is dead - long live SOAP

First of all do not write SOAP services if you can avoid it! It is over.

If you can not avoid it this package might help.

## Service

```go
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
				Bar: "Hello " + fooRequest.Foo,
			}
			response = fooResponse
			return
		},
	)
	err := soapServer.ListenAndServe(":8080")
	fmt.Println("exiting with error", err)
}

func main() {
	RunServer()
}
```

## Client

```go
package main

import (
	"encoding/xml"
	"log"

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

func main() {
	client := soap.NewClient("http://127.0.0.1:8080/", nil)
	response := &FooResponse{}
	httpResponse, err := client.Call("operationFoo", &FooRequest{Foo: "hello i am foo"}, response)
	if err != nil {
		panic(err)
	}
	log.Println(response.Bar, httpResponse.Status)
}
```

#  Apache License Version 2.0
