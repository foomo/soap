package soap

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"reflect"
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
	return xml.MarshalIndent(v, "", "	")
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
	url         string
	tls         bool
	auth        *BasicAuth
	tr          *http.Transport
	Marshaller  XMLMarshaller
	ContentType string
	SoapVersion string
}

// NewClient constructor. SOAP 1.1 is used by default. Switch to SOAP 1.2 with UseSoap12()
func NewClient(url string, auth *BasicAuth, tr *http.Transport) *Client {
	return &Client{
		url:         url,
		auth:        auth,
		tr:          tr,
		Marshaller:  newDefaultMarshaller(),
		ContentType: SoapContentType11, // default is SOAP 1.1
		SoapVersion: SoapVersion11,
	}
}

func (c *Client) UseSoap11() {
	c.SoapVersion = SoapVersion11
	c.ContentType = SoapContentType11
}

func (c *Client) UseSoap12() {
	c.SoapVersion = SoapVersion12
	c.ContentType = SoapContentType12
}

// Call make a SOAP call
func (c *Client) Call(soapAction string, request, response interface{}) (httpResponse *http.Response, err error) {

	envelope := Envelope{}

	envelope.Body.Content = request

	xmlBytes, err := c.Marshaller.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	// Adjust namespaces for SOAP 1.2
	if c.SoapVersion == SoapVersion12 {
		tmp := string(xmlBytes)
		tmp = strings.Replace(tmp, NamespaceSoap11, NamespaceSoap12, -1)
		xmlBytes = []byte(tmp)
	}
	//log.Println(string(xmlBytes))

	//l("SOAP Client Call() => Marshalled Request\n", string(xmlBytes))

	req, err := http.NewRequest("POST", c.url, bytes.NewBuffer(xmlBytes))
	if err != nil {
		return nil, err
	}
	if c.auth != nil {
		req.SetBasicAuth(c.auth.Login, c.auth.Password)
	}

	req.Header.Add("Content-Type", c.ContentType)
	req.Header.Set("User-Agent", UserAgent)

	if soapAction != "" {
		req.Header.Add("SOAPAction", soapAction)
	}

	req.Close = true
	tr := c.tr
	if tr == nil {
		tr = http.DefaultTransport.(*http.Transport)
	}
	client := &http.Client{Transport: tr}
	l("POST to", c.url, "with\n", string(xmlBytes))
	l("Header")
	LogJSON(req.Header)
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
		if !(strings.Contains(string(rawbody), "<soap") || strings.Contains(string(rawbody), "<SOAP")) {
			l("This is not a SOAP-Message: \n" + string(rawbody))
			return nil, errors.New("This is not a SOAP-Message: \n" + string(rawbody))
		}
		l("RAWBODY\n", string(rawbody))
	}

	// We have an empty body or a SOAP body
	l("\n\n## Response body:\n", string(rawbody))

	// Our structs for Envelope, Header, Body and Fault are tagged with namespace for SOAP 1.1
	// Therefore we must adjust namespaces for incoming SOAP 1.2 messages
	tmp := string(rawbody)
	tmp = strings.Replace(tmp, NamespaceSoap12, NamespaceSoap11, -1)
	rawbody = []byte(tmp)

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
		log.Println("soap/client.go Call(): COULD NOT UNMARSHAL\n", err)
	}

	// If we have a SOAP Fault, we return it as string in an error
	fault := respEnvelope.Body.Fault
	// If a SOAP Fault is received, try to jsonMarshal it and return it via the error.
	if fault != nil {
		return nil, errors.New("SOAP FAULT:\n" + formatFaultXML(rawbody, 1))
	}
	return
}

// Format the Soap Fault as indented string. Namespaces are dropped for better readability.
// Tags with lower level than start level is omitted
func formatFaultXML(xmlBytes []byte, startLevel int) string {
	indent := "	"
	d := xml.NewDecoder(bytes.NewBuffer([]byte(xmlBytes)))

	typeStart := reflect.TypeOf(xml.StartElement{})
	typeEnd := reflect.TypeOf(xml.EndElement{})
	typeCharData := reflect.TypeOf(xml.CharData{})

	level := 0
	out := bytes.NewBuffer([]byte(""))
	ind := func() {
		n := 0
		if level-startLevel-1 > 0 {
			n = level - startLevel - 1
		}
		out.Write([]byte(strings.Repeat(indent, n)))
	}
	lf := func() {
		out.Write([]byte("\n"))
	}

	lastWasStart := false
	lastWasCharData := false
	lastWasEnd := false

	for token, err := d.Token(); token != nil && err == nil; token, err = d.Token() {
		r := reflect.ValueOf(token)
		switch r.Type() {
		case typeStart:
			lastWasCharData = false
			se := token.(xml.StartElement)
			if lastWasEnd || lastWasStart {
				lf()
			}
			lastWasStart = true
			ind()
			elementName := se.Name.Local

			if level > startLevel {
				out.WriteString("<" + elementName)
				out.WriteString(">")
			}

			level++
			lastWasEnd = false
		case typeCharData:
			lastWasCharData = true
			_ = lastWasCharData
			lastWasStart = false
			cdata := token.(xml.CharData)
			xml.EscapeText(out, cdata)
			lastWasEnd = false
		case typeEnd:
			level--
			if lastWasEnd {
				lf()
				ind()
			}
			lastWasEnd = true
			lastWasStart = false
			end := token.(xml.EndElement)
			if level > startLevel {
				endTagName := end.Name.Local
				out.WriteString("</" + endTagName + ">")
			}

		}
	}
	return strings.Trim(string(out.Bytes()), " \n")
}
