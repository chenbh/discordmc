package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net"
	"time"
)

type packetType int32

const (
	TYPE_LOGIN    packetType = 3
	TYPE_CMD      packetType = 2
	TYPE_RESPONSE packetType = 0

	rconRetryDelay = 2 * time.Second
)

type packet struct {
	id      int32
	typ     packetType
	payload string
}

// TODO: multi packet responses
func readPacket(r io.Reader) (packet, error) {
	var (
		l int32
		p packet
	)
	binary.Read(r, binary.LittleEndian, &l)

	payloadLen := l - 4 - 4 // this wil contain the 2 null bytes
	buf := make([]byte, payloadLen)

	binary.Read(r, binary.LittleEndian, &p.id)
	binary.Read(r, binary.LittleEndian, &p.typ)
	binary.Read(r, binary.LittleEndian, buf)

	p.payload = string(buf[:payloadLen-2])
	return p, nil
}

func formatPacket(p packet) []byte {
	b := &bytes.Buffer{}
	binary.Write(b, binary.LittleEndian, int32(4+4+len(p.payload)+2))
	binary.Write(b, binary.LittleEndian, p.id)
	binary.Write(b, binary.LittleEndian, p.typ)
	binary.Write(b, binary.LittleEndian, []byte(p.payload+"\x00\x00"))

	return b.Bytes()
}

type rconClient struct {
	conn net.Conn
	addr string
	pass string
}

func newClient(addr, pass string) *rconClient {
	return &rconClient{nil, addr, pass}
}

func (c *rconClient) login() error {
	if c.conn != nil {
		return fmt.Errorf("already logged in")
	}

	conn, err := net.Dial("tcp", c.addr)
	if err != nil {
		return err
	}

	req := packet{
		id:      rand.Int31(),
		typ:     TYPE_LOGIN,
		payload: c.pass,
	}

	_, err = conn.Write(formatPacket(req))
	if err != nil {
		return fmt.Errorf("sending packet: %v", err)
	}

	res, err := readPacket(conn)
	if err != nil {
		return fmt.Errorf("receiving packet: %v", err)
	}

	if res.id != req.id {
		return fmt.Errorf("incorrect request id")
	}
	if res.typ != TYPE_CMD {
		return fmt.Errorf("failed to login")
	}

	c.conn = conn
	return nil
}

func (c *rconClient) command(cmd string) (string, error) {
	if c.conn == nil {
		return "", fmt.Errorf("must login first")
	}

	req := packet{
		id:      rand.Int31(),
		typ:     TYPE_CMD,
		payload: cmd,
	}

	_, err := c.conn.Write(formatPacket(req))
	if err != nil {
		return "", fmt.Errorf("sending packet: %v", err)
	}

	res, err := readPacket(c.conn)
	if err != nil {
		return "", fmt.Errorf("receiving packet: %v", err)
	}

	if res.id != req.id {
		return "", fmt.Errorf("incorrect request id")
	}
	if res.typ != TYPE_RESPONSE {
		return "", fmt.Errorf("invalid response")
	}

	return res.payload, nil
}

func (c *rconClient) Close() error {
	return c.conn.Close()
}
