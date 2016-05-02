package soap

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"strings"
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
		return nil, err
	}

	req, err := http.NewRequest("POST", s.url, bytes.NewBuffer(xmlBytes))
	if err != nil {
		return nil, err
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
		return nil, err
	}

	defer httpResponse.Body.Close()

	l("\n\n## Response header:\n", httpResponse.Header)

	mediaType, params, err := mime.ParseMediaType(httpResponse.Header.Get("Content-Type"))
	if err != nil {
		l("WARNING:", err)
	}
	l("MIMETYPE:", mediaType)
	var rawbody = []byte{}
	if strings.HasPrefix(mediaType, "multipart/") { // MULTIPART MESSAGE
		mr := multipart.NewReader(httpResponse.Body, params["boundary"])
		// If this is a multipart message, search for the soapy part
		foundSoap := false
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				return nil, err
			}
			if err != nil {
				return nil, err
			}
			slurp, err := ioutil.ReadAll(p)
			if err != nil {
				return nil, err
			}
			if strings.HasPrefix(string(slurp), "<soap") || strings.HasPrefix(string(slurp), "<SOAP") {
				rawbody = slurp
				foundSoap = true
				break
			}
		}
		if !foundSoap {
			return nil, errors.New("Multipart message does contain a soapy part.")
		}
	} else { // SINGLE PART MESSAGE
		rawbody, err = ioutil.ReadAll(httpResponse.Body)
		if err != nil {
			return httpResponse, err
		}
		// Check if there is a body and if yes if it's a soapy one.
		if len(rawbody) == 0 {
			l("INFO: Response Body is empty!")
			return // Empty responses are ok. Sometimes Sometimes only a Status 200 or 202 comes back
		}
		// There is a message body, but it's not SOAP. We cannot handle this!
		if !(strings.HasPrefix(string(rawbody), "<soap") || strings.HasPrefix(string(rawbody), "<SOAP")) {
			return nil, errors.New("This is not a SOAP-Message: \n" + string(rawbody))
		}

	}

	// We have an empty body or a SOAP body
	l("\n\n## Response body:\n", string(rawbody))
	respEnvelope := new(Envelope)
	type Dummy struct {
	}
	// Response struct may be nil, e.g. if only a Status 200 is expected.
	// In this case, we need a Dummy response to avoid a nil pointer if we receive a SOAP-Fault instead of the empty message (unmarshalling would fail)
	if response == nil {
		respEnvelope.Body = Body{Content: &Dummy{}}
	} else {
		respEnvelope.Body = Body{Content: response}
	}

	err = xml.Unmarshal(rawbody, respEnvelope)
	if err != nil {
		log.Println("soap/client.go Call(): COULD NOT UNMARSHAL")
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
