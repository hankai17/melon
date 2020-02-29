package main

import (
  "time"
  "net" // for parse address & listen & accept
  "log"
  "io" // for read/writer & copy
  "bytes" // for compat 1500 http_request
  "strings"
  "errors" // for errors.New
  "strconv" // for inta
  "net/http" // for parse http header
  "bufio" // for httpreader
  "net/url" // for construct http and recv
  "fmt"
  "encoding/binary"
  "github.com/shadowsocks/shadowsocks-go/shadowsocks" // https://www.jianshu.com/p/f688138cf465
)

const (
    readWait = 300 * time.Second
    writeWait = 300 * time.Second
)

// Laddr: local listen addr
// Proxy: forward tcp package to proxy
// Droxy: forward tcp package to daddr  Same as Proxy
type Melon struct {
  Laddr, Daddr, Proxy string
  Shadows   bool // for compare
}

func (g *Melon) Run() error {
  addr, err := net.ResolveTCPAddr("tcp", g.Laddr)
  if err != nil {
    return err
  }
  lfd, err := net.ListenTCP("tcp", addr)
  if err != nil {
    return err
  }

  for {
    conn, err := lfd.AcceptTCP()
    if err != nil {
      log.Println("accept:", err)
      continue
    }
    go g.handle(conn)
  }
  return lfd.Close()
}

// cli中 与dst握手 强制发510  先不管读到了什么  此时读c如果客户端发的是http铭文则构造成ss 发到dst 
// 如果客户端本身发的就是ss则透传 
func (g *Melon) cli(conn net.Conn) {
  lg := NewLog(true)
  defer func() {
    lg.Logln()
    lg.Flush()
  }()
  dconn, err := g.connect(g.Daddr)
  if err != nil {
    lg.Logln(err)
    return
  }
  defer dconn.Close()

  //laddr := dconn.(*net.TCPConn).LocalAddr().String()
  laddr := dconn.LocalAddr().String()
  lg.Logln(laddr)

  if _, err := dconn.Write([]byte{5, 1, 0}); err != nil {
    lg.Logln(err)
    return
  }
  lg.Logln(">>>|", []byte{5, 1, 0})

  if g.Shadows {
    lg.Logln("shadowsocks, aes-256-cfb")
    cipher, _ := shadowsocks.NewCipher("aes-256-cfb", "123456")
    conn = shadowsocks.NewConn(conn, cipher)
    addr, port, extra, err := getRequest(conn)
    if err != nil {
      lg.Logln(err)
      return
    }
    lg.Logln(addr, port)

    cmd := NewCmd(CmdConnect, AddrDomain, addr, port)
    if err = cmd.Write(dconn); err != nil {
      lg.Logln(err)
      return
    }
    lg.Logln(">>>|", cmd)
    if cmd, err = ReadCmd(dconn); err != nil {
      lg.Logln(err)
      return
    }
    lg.Logln("<<<|", cmd)
    if cmd.Cmd != Succeeded {
      conn.Write([]byte("HTTP/1.1 503 Service unavailable\r\n" + 
      "Proxy-Agent: gost/1.0\r\n\r\n"))
      return
    }

    if extra != nil {
      if _, err := dconn.Write(extra); err != nil {
        log.Println(err)
        return
      }
    }
    g.transport(conn, dconn)
    return
  }

  b := make([]byte, 8192)
  n, err := io.ReadFull(dconn, b[:2])
  if err != nil {
    lg.Logln(err)
    return
  }
  lg.Logln("<<<|", b[:n])

  n, err = conn.Read(b)
  if err != nil {
    lg.Logln(err)
    return
  }

  if b[0] == 5 { // ss5, NO AUTHENTICATION
    lg.Logln("|>>>", b[:n])

    if _, err := conn.Write([]byte{5, 0}); err != nil {
      lg.Logln(err)
      return
    }
    lg.Logln("|<<<", []byte{5, 0})
    cmd, err := ReadCmd(conn)
    if err != nil {
      lg.Logln(err)
      return
    }
    lg.Logln("|>>>", cmd)

    if err = cmd.Write(dconn); err != nil {
      lg.Logln(err)
      return
    }
    lg.Logln(">>>|", cmd)

    cmd, err = ReadCmd(dconn)
    if err != nil {
      lg.Logln(err)
      return
    }
    lg.Logln("<<<|", cmd)

    if err = cmd.Write(conn); err != nil {
      lg.Logln(err)
      return
    }
    lg.Logln("|<<<", cmd)
    g.transport(conn, dconn)
    return
  }

  req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(b[:n])))
  if err != nil {
    lg.Logln(err)
    return
  }
  lg.Logln(req.Method, req.RequestURI)

  var addr string
  var port uint16

  host := strings.Split(req.Host, ":")
  if len(host) == 1 {
    addr = host[0]
    port = 80
  }
  if len(host) == 2 {
    addr = host[0]
    n, _ := strconv.ParseUint(host[1], 10, 16)
    port = uint16(n)
  }
  cmd := NewCmd(CmdConnect, AddrDomain, addr, port)
  if err = cmd.Write(dconn); err != nil {
    lg.Logln(err)
    return
  }
  lg.Logln(">>>|", cmd)
  if cmd, err = ReadCmd(dconn); err != nil {
    lg.Logln(err)
    return
  }
  lg.Logln("<<<|", cmd)
  if cmd.Cmd != Succeeded {
    conn.Write([]byte("HTTP/1.1 503 Service unavailable\r\n" +
    "Proxy-Agent: melon/1.0.0\r\n\r\n"))
    return
  }
  if req.Method == "CONNECT" {
    if _, err = conn.Write(
      []byte("HTTP/1.1 200 Connection established\r\n" +
      "Proxy-Agent: melon/1.0.0\r\n\r\n")); err != nil {
        lg.Logln(err)
        return
      }
  } else {
    if err = req.Write(dconn); err != nil {
      lg.Logln(err)
      return
    }
  }
  g.transport(conn, dconn)

}

// 如果有代理则与代理建链 Connect请求200OK 没有代理直接建链
func (g *Melon) connect(addr string) (net.Conn, error) {
  if len(g.Proxy) == 0 { // 非代理比较简单直接去链接就好了
    taddr, err := net.ResolveTCPAddr("tcp", addr)
    if err != nil {
      return nil, err
    }
    return net.DialTCP("tcp", nil, taddr)
  }

  // 1 connect to proxy
  paddr, err := net.ResolveTCPAddr("tcp", g.Proxy)
  if err != nil {
    return nil, err
  }
  pconn, err := net.DialTCP("tcp", nil, paddr)
  if err != nil {
    return nil, err
  }

  // 2 send http CONNET request to proxy
  /*
  b := make([]byte, 1500)
  buffer := bytes.NewBuffer(b)
  buffer.WriteString("CONNECT " + addr + " HTTP/1.1\r\n")
  buffer.WriteString("Host: " + addr + "\r\n")
  buffer.WriteString("Proxy-Connection: keep-alive\r\n\r\n")
  if _, err = pconn.Write(buffer.Bytes()); err != nil {
    pconn.Close()
    return nil, err
  }
  */
  header := http.Header{}
  header.Set("Proxy-Connection", "keep-alive")
  req := &http.Request {
    Method: "CONNECTION",
    URL: &url.URL{Host: addr},
    Host: addr,
    Header: header,
  }
  if err := req.Write(pconn); err != nil {
    log.Println(err)
    pconn.Close()
    return nil, err
  }
  // 3 recv http resp from proxy
  /*
  r := ""
  for !strings.HasSuffix(r, "\r\n\r\n") {
    n := 0
    if n, err = pconn.Read(b); err != nil {
      pconn.Close()
      return nil, err
    }
    r += string(b[:n])
  }
  log.Println(r)
  if !strings.Contains(r, "200") {
    log.Println("connection failed:\n", r)
    err = errors.New(r)
    pconn.Close()
    return nil, err
  }
  */
  resp, err := http.ReadResponse(bufio.NewReader(pconn), req)
  if err != nil {
    log.Println(err)
    pconn.Close()
    return nil, err
  }
  if resp.StatusCode != http.StatusOK {
    pconn.Close()
    return nil, errors.New(resp.Status)
  }
  return pconn, nil
}

// srv服务ss 解析ss获取host 建链之 然后中转数据 
// 如果不是ss协议 则走http代理
func (g *Melon) srv(conn net.Conn) {
  b := make([]byte, 8192)
  lg := NewLog(true)
  defer func() {
    lg.Logln()
    lg.Flush()
  } ()
  n, err := conn.Read(b)
  if err != nil {
    lg.Logln(err)
    return
  }
  //if bytes.Equal(b[:n], []byte{5, 1, 0})  // ss5, NO AUTHENICATION
  if b[0] == 5 {
    lg.Logln("|>>>", b[:n])
    if _, err := conn.Write([]byte{5, 0}); err != nil {
      lg.Logln(err)
      return
    }
    lg.Logln("|<<<", []byte{5, 0})

    cmd, err := ReadCmd(conn)
    if err != nil {
      lg.Logln(err)
      return
    }
    lg.Logln("|>>>", cmd)
    host := cmd.Addr + ":" + strconv.Itoa(int(cmd.Port))
    lg.Logln("connect", host)
    tconn, err := g.connect(host)
    if err != nil {
      lg.Logln(err)
      cmd = NewCmd(ConnRefused, 0, "", 0)
      cmd.Write(conn)
      lg.Logln("|<<<", cmd)
      return
    }
    defer tconn.Close()
    if err = NewCmd(Succeeded, AddrIPv4, "0.0.0.0", 0).Write(conn); err != nil {
      lg.Logln(err)
      return
    }
    lg.Logln("|<<<", cmd)
    g.transport(conn, tconn)
    return
  }

  // curl -vx 127.0.0.1:8080 -o /dev/null "http://www.ifeng.com"
  // curl -v -o /dev/null "http://127.0.0.1:8080/1.html" -H "Host: www.ifeng.com"  ALL is well
  req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(b[:n])))
  if err != nil {
    lg.Logln(err)
    return
  }
  host := req.Host
  if !strings.Contains(host, ":") {
    host = host + ":80"
  }
  log.Println(req.Method, host, req.RequestURI)
  tconn, err := g.connect(host)
  if err != nil {
    lg.Logln(err)
    conn.Write([]byte("HTTP/1.1 503 Service unavailable\r=n" +
    "Proxy-Agent: melon/1.0.0\r\n\r\n"))
    return
  }
  defer tconn.Close()
  if req.Method == "CONNECT" {
    if _, err = conn.Write([]byte("HTTP/1.1 200 Connection established\r\n" +
    "Proxy-Agent: melon/1.0.0\r\n\r\n")); err != nil {
      lg.Logln(err)
      return
    }
  } else {
    if err := req.Write(tconn); err != nil {
      lg.Logln(err)
      return
    }
  }
  g.transport(conn, tconn)
}

func getRequest(conn net.Conn) (host string, port uint16, extra []byte, err error) {
  const (
    idType = 0
    idIP0  = 1
    idDmLen = 1
    idDm0   = 2
    typeIPv4 = 1
    typeDm = 3
    typeIPv6 = 4
    lenIPv4 = 1 + net.IPv4len + 2
    lenIPv6 = 1 + net.IPv6len + 2
    lenDmBase = 1 + 1 + 2
  )
  buf := make([]byte, 260)
  var n int
  if n, err = io.ReadAtLeast(conn, buf, idDmLen+1); err != nil {
    log.Println(err)
    return
  }
  log.Println(buf[:n])
  reqLen := -1
  switch buf[idType] {
  case typeIPv4:
    reqLen = lenIPv4
  case typeIPv6:
    reqLen = lenIPv6
  case typeDm:
    reqLen = int(buf[idDmLen]) + lenDmBase
  default:
    err = fmt.Errorf("addr type %d not supported", buf[idType])
    return
  }
  if n < reqLen {
    if _, err = io.ReadFull(conn, buf[n:reqLen]); err != nil {
      log.Println(err)
      return
    }
  } else if n > reqLen {
    extra = buf[reqLen:n]
  }

  switch buf[idType] {
  case typeIPv4:
    host = net.IP(buf[idIP0 : idIP0+net.IPv4len]).String()
  case typeIPv6:
    host = net.IP(buf[idIP0 : idIP0+net.IPv6len]).String()
  case typeDm:
    host = string(buf[idDm0 : idDm0+buf[idDmLen]])
  }
  port = binary.BigEndian.Uint16(buf[reqLen-1 : reqLen])
  return
}

func (g *Melon) handle(conn net.Conn) {
  defer conn.Close()
  // as client
  if  len(g.Daddr) > 0 {
    g.cli(conn)
    return
  }

  // as server
  g.srv(conn)
}

func (g *Melon) connectDst(sconn net.Conn) {
  dconn, err := g.connect(g.Daddr)
  if err != nil {
    return
  }
  defer dconn.Close()
  g.transport(sconn, dconn)
  return
}

func (g *Melon) pipe(src io.Reader, dst io.Writer, c chan<- error) {
  _, err := io.Copy(dst, src) // https://www.cnblogs.com/smartrui/p/12110576.html
  c <- err // send err. error may eof
}

func (g *Melon) forward(conn, pconn net.Conn) error {
  defer pconn.Close()
  _, err := net.ResolveTCPAddr("tcp", g.Daddr)
  if err != nil {
    log.Println(err)
  }
  // TODO 
  return nil
}

func (g *Melon) transport(conn, dconn net.Conn) (err error) {
  rChan := make(chan error)
  wChan := make(chan error)

  go g.pipe(conn, dconn, wChan)
  go g.pipe(dconn, conn, rChan)

  select {
  case err = <-wChan:
  case err = <-rChan:
  }
  return
}
