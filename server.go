package soap

type Server struct {
}

func NewServer(addr string) *Server {
	s := &Server{}
	return s
}

func (s *Server) HandleOperation(operationName string, requestFactory func() interface{}, operationHandler func(request interface{}) (response interface{}, err error)) {

}
