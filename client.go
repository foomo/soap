package soap

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"time"
)

// ClientDialTimeout default timeout 30s
var ClientDialTimeout = time.Duration(30 * time.Second)

// UserAgent is the default user agent
var UserAgent = "go-soap-0.1"

// XMLMarshaller lets you inject your favourite custom xml implementation
type XMLMarshaller interface {
	Marshal(v interface{}) ([]byte, error)
	Unmarshal(xml []byte, v interface{}) error
}

type defaultMarshaller struct {
}

func (dm *defaultMarshaller) Marshal(v interface{}) (xmlBytes []byte, err error) {
	return xml.Marshal(v)
}

func (dm *defaultMarshaller) Unmarshal(xmlBytes []byte, v interface{}) error {
	return xml.Unmarshal(xmlBytes, v)
}

func newDefaultMarshaller() XMLMarshaller {
	return &defaultMarshaller{}
}

func dialTimeout(network, addr string) (net.Conn, error) {
	return net.DialTimeout(network, addr, ClientDialTimeout)
}

// BasicAuth credentials for the client
type BasicAuth struct {
	Login    string
	Password string
}

// Client generic SOAP client
type Client struct {
	url        string
	tls        bool
	auth       *BasicAuth
	tr         *http.Transport
	Marshaller XMLMarshaller
}

// NewClient constructor
func NewClient(url string, auth *BasicAuth, tr *http.Transport) *Client {
	return &Client{
		url:        url,
		auth:       auth,
		tr:         tr,
		Marshaller: newDefaultMarshaller(),
	}
}

// Call make a SOAP call
func (s *Client) Call(soapAction string, request, response interface{}) (httpResponse *http.Response, err error) {

	envelope := Envelope{}

	envelope.Body.Content = request

	xmlBytes, err := s.Marshaller.Marshal(envelope)
	if err != nil {
		panic(err)
	}

	req, err := http.NewRequest("POST", s.url, bytes.NewBuffer(xmlBytes))
	if err != nil {
		panic(err)
	}
	if s.auth != nil {
		req.SetBasicAuth(s.auth.Login, s.auth.Password)
	}

	req.Header.Add("Content-Type", SOAPContentType)
	req.Header.Set("User-Agent", UserAgent)

	if soapAction != "" {
		req.Header.Add("SOAPAction", soapAction)
	}

	req.Close = true
	tr := s.tr
	if tr == nil {
		tr = http.DefaultTransport.(*http.Transport)
	}
	client := &http.Client{Transport: tr}
	l("POST to", s.url, "with\n", string(xmlBytes))
	httpResponse, err = client.Do(req)
	if err != nil {
		panic(err)
	}

	defer httpResponse.Body.Close()

	rawbody, err := ioutil.ReadAll(httpResponse.Body)
	if err != nil {
		panic(err)
	}
	if len(rawbody) == 0 {
		l("empty response")
		return
	}

	l("response", string(rawbody))
	respEnvelope := new(Envelope)
	type Dummy struct {
	}
	if response == nil {
		// This may be the case, if an empty response is expected, but a SOAP Fault is received.
		// In this case unmarshalling of the rawbody would fail without using the Dummy, because response being a nil-pointer.
		respEnvelope.Body = Body{Content: &Dummy{}}
	} else {
		respEnvelope.Body = Body{Content: response}
	}

	err = xml.Unmarshal(rawbody, respEnvelope)
	if err != nil {
		log.Println("soap/client.go Call(): COULD NOT UNMARSHAL")
		panic(err)
	}

	fault := respEnvelope.Body.Fault
	// If a SOAP Fault is received, try to jsonMarshal it and return it via the error.
	if fault != nil {
		log.Println("Received SOAP FAULT")
		jsonBytes, err := json.MarshalIndent(respEnvelope.Body.Fault, "", "	")
		if err != nil {
			log.Println("soap/client.go Call(): could not jsonMarhal SOAP-Fault")
			return httpResponse, err
		}
		log.Println(string(jsonBytes))
		return httpResponse, errors.New("Received SOAP Fault:\n" + string(jsonBytes))

	}
	return
}
