package soap

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
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
	if Verbose {
		l("writing response: " + string(b))
	}
	return w.w.Write(b)
}

func (w *responseWriter) WriteHeader(code int) {
	w.w.WriteHeader(code)
}

// Server a SOAP server, which can be run standalone or used as a http.HandlerFunc
type Server struct {
	handlers    map[string]map[string]map[string]*operationHander
	Marshaller  XMLMarshaller
	ContentType string
	SoapVersion string
}

// NewServer construct a new SOAP server
func NewServer() *Server {
	s := &Server{
		handlers:    make(map[string]map[string]map[string]*operationHander),
		Marshaller:  newDefaultMarshaller(),
		ContentType: SoapContentType11,
		SoapVersion: SoapVersion11,
	}
	return s
}

func (s *Server) UseSoap11() {
	s.SoapVersion = SoapVersion11
	s.ContentType = SoapContentType11
}

func (s *Server) UseSoap12() {
	s.SoapVersion = SoapVersion12
	s.ContentType = SoapContentType12
}

// RegisterHandler register to handle an operation
func (s *Server) RegisterHandler(path string, action string, messageType string, requestFactory RequestFactoryFunc, operationHandlerFunc OperationHandlerFunc) {
	_, pathHandlersOK := s.handlers[path]
	if !pathHandlersOK {
		s.handlers[path] = make(map[string]map[string]*operationHander)
	}
	_, ok := s.handlers[path][action]
	if !ok {
		s.handlers[path][action] = make(map[string]*operationHander)
	}
	s.handlers[path][action][messageType] = &operationHander{
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
		addSOAPHeader(w, len(xmlBytes), s.ContentType)
		w.Write(xmlBytes)
	}
}

// WriteHeader first sets header like content-type and then writes the header
func (s *Server) WriteHeader(w http.ResponseWriter, code int) {
	setContentType(w, s.ContentType)
	w.WriteHeader(code)
}

func setContentType(w http.ResponseWriter, contentType string) {
	w.Header().Set("Content-Type", contentType)
}

func addSOAPHeader(w http.ResponseWriter, contentLength int, contentType string) {
	setContentType(w, contentType)
	w.Header().Set("Content-Length", fmt.Sprint(contentLength))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	soapAction := r.Header.Get("SOAPAction")
	l("ServeHTTP method:", r.Method, ", path:", r.URL.Path, ", SOAPAction", "\""+soapAction+"\"")
	// we have a valid request time to call the handler
	w = &responseWriter{
		w:             w,
		outputStarted: false,
	}
	switch r.Method {
	case "POST":
		l("incoming POST")
		soapRequestBytes, err := ioutil.ReadAll(r.Body)
		// Our structs for Envelope, Header, Body and Fault are tagged with namespace for SOAP 1.1
		// Therefore we must adjust namespaces for incoming SOAP 1.2 messages
		if s.SoapVersion == SoapVersion12 {
			tmp := string(soapRequestBytes)
			tmp = strings.Replace(tmp, NamespaceSoap12, NamespaceSoap11, -1)
			soapRequestBytes = []byte(tmp)
		}

		if err != nil {
			s.handleError(errors.New("could not read POST:: "+err.Error()), w)
			return
		}
		pathHandlers, pathHandlerOK := s.handlers[r.URL.Path]
		if !pathHandlerOK {
			s.handleError(errors.New("unknown path"), w)
			return
		}
		actionHandlers, ok := pathHandlers[soapAction]
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

		err = s.Marshaller.Unmarshal(soapRequestBytes, &probeEnvelope)
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
			Header: Header{},
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

		response, err := actionHandler.handler(request, w, r)
		if err != nil {
			l("action handler threw up")
			s.handleError(err, w)
			return
		}
		l("result", jsonDump(response))
		if !w.(*responseWriter).outputStarted {
			responseEnvelope := &Envelope{
				Body: Body{
					Content: response,
				},
			}
			xmlBytes, err := s.Marshaller.Marshal(responseEnvelope)
			// Adjust namespaces for SOAP 1.2
			if s.SoapVersion == SoapVersion12 {
				tmp := string(xmlBytes)
				tmp = strings.Replace(tmp, NamespaceSoap11, NamespaceSoap12, -1)
				xmlBytes = []byte(tmp)
			}
			if err != nil {
				s.handleError(errors.New("could not marshal response:: "+err.Error()), w)
			}
			addSOAPHeader(w, len(xmlBytes), s.ContentType)
			w.Write(xmlBytes)
		} else {
			l("action handler sent its own output")
		}

	default:
		// this will be a soap fault !?
		s.handleError(errors.New("this is a soap service - you have to POST soap requests"), w)
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
