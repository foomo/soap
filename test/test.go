package main

import (
	"fmt"

	"github.com/foomo/soap"
)

type FooRequest struct {
	Foo string
}

type FooResponse struct {
	Bar string
}

func RunServer() {
	soapServer := soap.NewServer()
	/*
		soapServer.HandleOperation(
			"operationFoo",
			"FooRequest",
			func() interface{} {
				return &FooRequest{}
			},
			func(request interface{}) (response interface{}, err error) {
				fooRequest := request.(*FooRequest)
				fooResponse := &FooResponse{
					Bar: "Hello " + fooRequest.Foo,
				}
				response = fooResponse
				return
			},
		)
	*/
	err := soapServer.ListenAndServe(":8080")
	fmt.Println(err)
}

func main() {
	RunServer()
}
