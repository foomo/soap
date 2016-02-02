package soap

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"io/ioutil"
	"net/http"
)

// OperationHandlerFunc runs the actual business logic - request is whatever you constructed in RequestFactoryFunc
type OperationHandlerFunc func(request interface{}, w http.ResponseWriter, httpRequest *http.Request) (response interface{}, err error)

// RequestFactoryFunc constructs a request object for OperationHandlerFunc
type RequestFactoryFunc func() interface{}

type dummyContent struct{}

type operationHander struct {
	requestFactory RequestFactoryFunc
	handler        OperationHandlerFunc
}

type responseWriter struct {
	w             http.ResponseWriter
	outputStarted bool
}

func (w *responseWriter) Header() http.Header {
	return w.w.Header()
}
func (w *responseWriter) Write(b []byte) (int, error) {
	w.outputStarted = true
	return w.w.Write(b)
}

func (w *responseWriter) WriteHeader(code int) {
	w.w.WriteHeader(code)
}

// Server a SOAP server, which can be run standalone or used as a http.HandlerFunc
type Server struct {
	handlers   map[string]map[string]*operationHander
	Marshaller XMLMarshaller
}

// NewServer construct a new SOAP server
func NewServer() *Server {
	s := &Server{
		handlers:   make(map[string]map[string]*operationHander),
		Marshaller: newDefaultMarshaller(),
	}
	return s
}

// HandleOperation register to handle an operation
func (s *Server) HandleOperation(action string, messageType string, requestFactory RequestFactoryFunc, operationHandlerFunc OperationHandlerFunc) {
	_, ok := s.handlers[action]
	if !ok {
		s.handlers[action] = make(map[string]*operationHander)
	}
	s.handlers[action][messageType] = &operationHander{
		handler:        operationHandlerFunc,
		requestFactory: requestFactory,
	}
}

func (s *Server) handleError(err error, w http.ResponseWriter) {
	// has to write a soap fault
	l("handling error:", err)
	responseEnvelope := &Envelope{
		Body: Body{
			Content: &Fault{
				String: err.Error(),
			},
		},
	}
	xmlBytes, xmlErr := s.Marshaller.Marshal(responseEnvelope)
	if xmlErr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("could not marshal soap fault for: " + err.Error() + " xmlError: " + xmlErr.Error()))
	} else {
		w.Write(xmlBytes)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	switch r.Method {
	case "POST":
		l("incoming POST")
		soapRequestBytes, err := ioutil.ReadAll(r.Body)

		if err != nil {
			s.handleError(errors.New("could not read POST:: "+err.Error()), w)
			return
		}

		soapAction := r.Header.Get("SOAPAction")
		l("SOAPAction", "\""+soapAction+"\"")
		actionHandlers, ok := s.handlers[soapAction]
		if !ok {
			s.handleError(errors.New("unknown action \""+soapAction+"\""), w)
			return
		}

		// we need to find out, what is in the body
		probeEnvelope := &Envelope{
			Body: Body{
				Content: &dummyContent{},
			},
		}

		err = xml.Unmarshal(soapRequestBytes, &probeEnvelope)
		if err != nil {
			s.handleError(errors.New("could not probe soap body content:: "+err.Error()), w)
			return
		}

		t := probeEnvelope.Body.SOAPBodyContentType
		l("found content type", t)
		actionHandler, ok := actionHandlers[t]
		if !ok {
			s.handleError(errors.New("no action handler for content type: \""+t+"\""), w)
			return
		}
		request := actionHandler.requestFactory()
		envelope := &Envelope{
			Body: Body{
				Content: request,
			},
		}

		err = xml.Unmarshal(soapRequestBytes, &envelope)
		if err != nil {
			s.handleError(errors.New("could not unmarshal request:: "+err.Error()), w)
			return
		}
		l("request", jsonDump(envelope))

		// we have a valid request time to call the handler
		responseWriter := &responseWriter{
			w:             w,
			outputStarted: false,
		}
		response, err := actionHandler.handler(request, responseWriter, r)
		if err != nil {
			l("action handler threw up")
			s.handleError(err, w)
			return
		}
		l("result", jsonDump(response))
		if !responseWriter.outputStarted {
			responseEnvelope := &Envelope{
				Body: Body{
					Content: response,
				},
			}
			xmlBytes, err := s.Marshaller.Marshal(responseEnvelope)
			if err != nil {
				s.handleError(errors.New("could not marshal response:: "+err.Error()), w)
			}
			w.Write(xmlBytes)
		} else {
			l("action handler sent its own output")
		}

	default:
		// this will be a soap fault !?
		w.Write([]byte("this is a soap service - you have to POST soap requests\n"))
		w.Write([]byte("invalid method: " + r.Method))
	}
}

func jsonDump(v interface{}) string {
	if !Verbose {
		return "not dumping"
	}
	jsonBytes, err := json.MarshalIndent(v, "", "	")
	if err != nil {
		return "error in json dump :: " + err.Error()
	}
	return string(jsonBytes)
}

// ListenAndServe run standalone
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s)
}
