package proxy

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"time"

	"github.com/techievee/ethash-mining-pool/util"
)

const (
	MaxSSLReqSize = 1024
)

func (s *ProxyServer) ListenSSLTCP() {
	timeout := util.MustParseDuration(s.config.Proxy.StratumSSL.Timeout)
	s.timeout = timeout

	cert, err := tls.LoadX509KeyPair(s.config.Proxy.StratumSSL.CertFile, s.config.Proxy.StratumSSL.CertKey)
	if err != nil {
		log.Fatalf("Error loading certificate. ", err)
	}

	tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}}

	addr, err := net.ResolveTCPAddr("tcp4", s.config.Proxy.StratumSSL.Listen)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	server, err := tls.Listen("tcp", addr.String(), tlsCfg)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	defer server.Close()

	log.Printf("SSL Stratum listening on %s", s.config.Proxy.StratumSSL.Listen)
	var accept = make(chan int, s.config.Proxy.StratumSSL.MaxConn)
	n := 0

	for {
		sslconn, err := server.Accept()
		if err != nil {
			continue
		}
		//conn.SetKeepAlive(true)

		ip, _, _ := net.SplitHostPort(sslconn.RemoteAddr().String())

		if s.policy.IsBanned(ip) || !s.policy.ApplyLimitPolicy(ip) {
			log.Fatalf("Banned : %v", ip)
			sslconn.Close()
			continue
		}

		n += 1
		cs := &Session{sslconn: sslconn, ip: ip}

		accept <- n
		go func(cs *Session) {
			err = s.handleSSLTCPClient(cs)
			if err != nil {
				s.removeSSLSession(cs)
				sslconn.Close()
			}
			<-accept
		}(cs)
	}
}

func (s *ProxyServer) handleSSLTCPClient(cs *Session) error {
	cs.enc = json.NewEncoder(cs.sslconn)
	connbuff := bufio.NewReaderSize(cs.sslconn, MaxSSLReqSize)
	s.setSSLDeadline(cs.sslconn)

	for {
		data, isPrefix, err := connbuff.ReadLine()
		if isPrefix {
			log.Printf("Socket flood detected from %s", cs.ip)
			s.policy.BanClient(cs.ip)
			return err
		} else if err == io.EOF {
			log.Printf("Client %s disconnected", cs.ip)
			s.removeSSLSession(cs)
			break
		} else if err != nil {
			log.Printf("Error reading from socket: %v", err)
			return err
		}

		if len(data) > 1 {
			var req StratumReq
			err = json.Unmarshal(data, &req)
			if err != nil {
				s.policy.ApplyMalformedPolicy(cs.ip)
				log.Printf("Malformed stratum request from %s: %v", cs.ip, err)
				return err
			}
			s.setSSLDeadline(cs.sslconn)
			err = cs.handleSSLTCPMessage(s, &req)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (cs *Session) handleSSLTCPMessage(s *ProxyServer, req *StratumReq) error {
	// Handle RPC methods
	switch req.Method {
	case "eth_submitLogin":
		var params []string
		err := json.Unmarshal(req.Params, &params)
		if err != nil {
			log.Println("Malformed stratum request params from", cs.ip)
			return err
		}
		reply, errReply := s.handleLoginRPC(cs, params, req.Worker)
		if errReply != nil {
			return cs.sendSSLTCPError(req.Id, errReply)
		}
		return cs.sendSSLTCPResult(req.Id, reply)
	case "eth_getWork":
		reply, errReply := s.handleGetWorkRPC(cs)
		if errReply != nil {
			return cs.sendSSLTCPError(req.Id, errReply)
		}
		return cs.sendSSLTCPResult(req.Id, &reply)
	case "eth_submitWork":
		var params []string
		err := json.Unmarshal(req.Params, &params)
		if err != nil {
			log.Println("Malformed stratum request params from", cs.ip)
			return err
		}
		reply, errReply := s.handleTCPSubmitRPC(cs, req.Worker, params)
		if errReply != nil {
			return cs.sendSSLTCPError(req.Id, errReply)
		}
		return cs.sendSSLTCPResult(req.Id, &reply)
	case "eth_submitHashrate":
		return cs.sendSSLTCPResult(req.Id, true)
	default:
		errReply := s.handleUnknownRPC(cs, req.Method)
		return cs.sendSSLTCPError(req.Id, errReply)
	}
}

func (cs *Session) sendSSLTCPResult(id json.RawMessage, result interface{}) error {
	cs.Lock()
	defer cs.Unlock()

	message := JSONRpcResp{Id: id, Version: "2.0", Error: nil, Result: result}
	return cs.enc.Encode(&message)
}

func (cs *Session) pushSSLNewJob(result interface{}) error {
	cs.Lock()
	defer cs.Unlock()
	// FIXME: Temporarily add ID for Claymore compliance
	message := JSONPushMessage{Version: "2.0", Result: result, Id: 0}
	return cs.enc.Encode(&message)
}

func (cs *Session) sendSSLTCPError(id json.RawMessage, reply *ErrorReply) error {
	cs.Lock()
	defer cs.Unlock()

	message := JSONRpcResp{Id: id, Version: "2.0", Error: reply}
	err := cs.enc.Encode(&message)
	if err != nil {
		return err
	}
	return errors.New(reply.Message)
}

func (self *ProxyServer) setSSLDeadline(sslconn net.Conn) {
	sslconn.SetDeadline(time.Now().Add(self.timeout))
}

func (s *ProxyServer) registerSSLSession(cs *Session) {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	s.sessions[cs] = struct{}{}
}

func (s *ProxyServer) removeSSLSession(cs *Session) {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	delete(s.sessions, cs)
}

func (s *ProxyServer) broadcastSSLNewJobs() {
	t := s.currentBlockTemplate()
	if t == nil || len(t.Header) == 0 || s.isSick() {
		return
	}
	reply := []string{t.Header, t.Seed, s.diff}

	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()

	count := len(s.sessions)
	log.Printf("Broadcasting new job to %v stratum miners", count)

	start := time.Now()
	bcast := make(chan int, 1024)
	n := 0

	for m, _ := range s.sessions {
		n++
		bcast <- n

		go func(cs *Session) {
			err := cs.pushSSLNewJob(&reply)
			<-bcast
			if err != nil {
				log.Printf("Job transmit error to %v@%v: %v", cs.login, cs.ip, err)
				s.removeSSLSession(cs)
			} else {
				s.setSSLDeadline(cs.sslconn)
			}
		}(m)
	}
	log.Printf("Jobs broadcast finished %s", time.Since(start))
}
