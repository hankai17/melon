package main

import (
  "encoding/binary" // used for edian
  "errors" // used for macros
  "io" // used for read/writer
  "net" // used for ip parser
)

const (
  Ver5 = 5
)

const (
  AddrIPv4 uint8 = 1
  AddrDomain     = 2
  AddrIPv6 uint8 = 3
)

const (
  Succeeded uint8 = iota
  Failure
  NotAllowed
  NetUnreachable
  HostUnreachable
  ConnRefused
  TTLExpired
  CmdUnsupported
  AddrUnsupported
)

var (
  ErrBadVersion = errors.New("Bad version")
  ErrBadFormat = errors.New("Bad format")
  ErrBadAddrType = errors.New("Bad Addr type")
  ErrShortBuffer = errors.New("Short buffer")

  cmdErrMap = map[uint8] error {
    Failure:        errors.New("General SOCKS server failure"),
    NotAllowed:     errors.New("Connection not allowed by releset"),
    NetUnreachable: errors.New("Network unreachable"),
    HostUnreachable:errors.New("Host unreachable"),
    ConnRefused:    errors.New("Connection refused"),
    TTLExpired:     errors.New("TTL expired"),
    CmdUnsupported: errors.New("Command not support"),
    AddrUnsupported:errors.New("Address type not support"),
  }
)

/*
+----+-----+-------+------+----------+----------+
|VER | CMD |  RSV  | ATYP | DST.ADDR | DST.PORT |
+----+-----+-------+------+----------+----------+
| 1  |  1  | X'00' |  1   | Variable |    2     |
+----+-----+-------+------+----------+----------+
*/

type Cmd struct {
	Cmd      uint8
	AddrType uint8
	Addr     string
	Port     uint16
}

func NewCmd(cmd uint8, atype uint8, addr string, port uint16) *Cmd {
  return &Cmd{
    Cmd:        cmd,
    AddrType:   atype,
    Addr:       addr,
    Port:       port,
  }
}

func ReadCmd(r io.Reader) (*Cmd, error) {
  b := make([]byte, 256)
  n, err := r.Read(b) // same as ats's do_io_read(MIOBuffer)
  if err != nil {
    return nil, err
  }
  if n < 10 {
    return nil, ErrBadFormat
  }
  if b[0] != Ver5 {
    return nil, ErrBadVersion
  }
  cmd := &Cmd {
    Cmd:      b[1],
    AddrType: b[3],
  }
  pos := 4
  switch cmd.AddrType {
  case AddrIPv4:
    if n != 10 {
      return nil, ErrBadFormat
    }
    cmd.Addr = net.IP(b[pos : pos + 4]).String()
    pos += 4
  case AddrIPv6:
    if n != 22 {
      return nil, ErrBadFormat
    }
    cmd.Addr = net.IP(b[pos : pos + 16]).String()
    pos += 16
  case AddrDomain:
    length := int(b[pos])
    if n != 4 + 1 + length + 2 {
      return nil, ErrBadFormat
    }
    pos++
    cmd.Addr = string(b[pos: pos+length])
    pos += length
  default:
    return nil, ErrBadAddrType
  }
  cmd.Port = binary.BigEndian.Uint16(b[pos:])
  return cmd, nil
}

func (cmd *Cmd) Write(w io.Writer) (err error) {
  b := make([]byte, 256)
  b[0] = Ver5
  b[1] = cmd.Cmd
  b[3] = cmd.AddrType
  pos := 4
  switch cmd.AddrType {
  case AddrIPv4:
    pos += copy(b[pos:], net.ParseIP(cmd.Addr).To4()) // https://golang.google.cn/pkg/net/#IP // host order
  case AddrIPv6:
    pos += copy(b[pos:], net.ParseIP(cmd.Addr).To16())
  case AddrDomain:
    b[pos] = byte(len(cmd.Addr))
    pos++
    pos += copy(b[pos:], []byte(cmd.Addr)) // deep copy 
  }
  binary.BigEndian.PutUint16(b[pos:], cmd.Port) // port is net order
  pos += 2
  _, err = w.Write(b[:pos])
  return
}

func (cmd *Cmd) GetError() error {
  return cmdErrMap[cmd.Cmd]
}

