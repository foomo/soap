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
	soap.Verbose = true
	client := soap.NewClient("http://127.0.0.1:8080/", nil, nil)
	response := &FooResponse{}
	httpResponse, err := client.Call("operationFoo", &FooRequest{Foo: "hello i am foo"}, response)
	if err != nil {
		panic(err)
	}
	log.Println(response.Bar, httpResponse.Status)
}
