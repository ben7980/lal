package rtmp

import (
	"github.com/q191201771/lal/pkg/util/log"
	"net"
	"sync"
)

type ServerObserver interface {
	NewRTMPPubSessionCB(session *ServerSession, group *Group) bool // 返回true则允许推流，返回false则强制关闭这个连接
	NewRTMPSubSessionCB(session *ServerSession, group *Group) bool // 返回true则允许拉流，返回false则强制关闭这个连接
}

type Server struct {
	obs  ServerObserver
	addr string
	ln   net.Listener

	groupMap map[string]*Group
	mutex    sync.Mutex
}

func NewServer(obs ServerObserver, addr string) *Server {
	return &Server{
		obs:      obs,
		addr:     addr,
		groupMap: make(map[string]*Group),
	}
}

func (server *Server) RunLoop() error {
	var err error
	server.ln, err = net.Listen("tcp", server.addr)
	if err != nil {
		return err
	}
	log.Infof("start rtmp listen. addr=%s", server.addr)
	for {
		conn, err := server.ln.Accept()
		if err != nil {
			return err
		}
		go server.handleConnect(conn)
	}
}

func (server *Server) Dispose() {
	if err := server.ln.Close(); err != nil {
		log.Error(err)
	}
}

func (server *Server) handleConnect(conn net.Conn) {
	log.Infof("accept a rtmp connection. remoteAddr=%v", conn.RemoteAddr())
	session := NewServerSession(server, conn)
	err := session.RunLoop()
	log.Infof("close a rtmp session.type=%d, err=%v", session.t, err)
	switch session.t {
	case ServerSessionTypeUnknown:
	// noop
	case ServerSessionTypePub:
		server.DelRTMPPubSession(session)
	case ServerSessionTypeSub:
		server.DelRTMPSubSession(session)
	}
}

func (server *Server) getOrCreateGroup(appName string, streamName string) *Group {
	server.mutex.Lock()
	defer server.mutex.Unlock()

	group, exist := server.groupMap[streamName]
	if !exist {
		group = NewGroup(appName, streamName)
		server.groupMap[streamName] = group
	}
	go group.RunLoop()
	return group
}

// ServerSessionObserver
func (server *Server) NewRTMPPubSessionCB(session *ServerSession) {
	group := server.getOrCreateGroup(session.AppName, session.StreamName)

	if !server.obs.NewRTMPPubSessionCB(session, group) {
		log.Warnf("dispose PubSession since pub exist.")
		session.Dispose()
		return
	}
	group.AddPubSession(session)
}

// ServerSessionObserver
func (server *Server) NewRTMPSubSessionCB(session *ServerSession) {
	group := server.getOrCreateGroup(session.AppName, session.StreamName)

	if !server.obs.NewRTMPSubSessionCB(session, group) {
		// TODO chef: 关闭这个连接
		return
	}
	group.AddSubSession(session)
}

func (server *Server) DelRTMPPubSession(session *ServerSession) {
	group := server.getOrCreateGroup(session.AppName, session.StreamName)

	// TODO chef: obs

	group.DelPubSession(session)
}

func (server *Server) DelRTMPSubSession(session *ServerSession) {
	group := server.getOrCreateGroup(session.AppName, session.StreamName)

	// TODO chef: obs

	group.DelSubSession(session)
}
