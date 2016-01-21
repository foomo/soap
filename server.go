package soap

import (
	"errors"
	"net/http"
)

type OperationHandlerFunc func(request interface{}) (response interface{}, err error)
type RequestFactoryFunc func() interface{}

type operationHander struct {
	requestFactory RequestFactoryFunc
	handler        OperationHandlerFunc
}

type Server struct {
	handlers map[string]map[string]*operationHander
}

func NewServer() *Server {
	s := &Server{
		handlers: make(map[string]map[string]*operationHander),
	}
	return s
}

// HandleOperation register to handle an operation
func (s *Server) HandleOperation(action string, messageType string, requestFactory RequestFactoryFunc, operationHandlerFunc OperationHandlerFunc) {
	s.handlers[action][messageType] = &operationHander{
		handler:        operationHandlerFunc,
		requestFactory: requestFactory,
	}
}

func (s *Server) serveSOAP(requestEnvelopeBytes []byte, soapAction string) (responseEnvelopeBytes []byte, err error) {
	messageType := "find me as the element name in the soap body"
	actionHandlers, ok := s.handlers[soapAction]
	if !ok {
		err = errors.New("could not find handlers for action: \"" + soapAction + "\"")
		return
	}
	handler, ok := actionHandlers[messageType]
	if !ok {
		err = errors.New("no handler for message type: " + messageType)
		return
	}
	request := handler.requestFactory()
	// parse from envelope.body.content into request
	response, err := handler.handler(request)
	responseEnvelope := &SOAPEnvelope{
		Body: SOAPBody{},
	}
	if err != nil {
		// soap fault party time
		responseEnvelope.Body.Fault = &SOAPFault{
			String: err.Error(),
		}
	} else {
		responseEnvelope.Body.Content = response
	}
	// marshal responseEnvelope
	return
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		w.Write([]byte("that actually could be a soap request"))
	default:
		// this will be a soap fault !?
		w.Write([]byte("this is a soap service - you have to POST soap requests\n"))
		w.Write([]byte("invalid method: " + r.Method))
	}
}

func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s)
}
