package main

import (
  "time"
  "net" // for parse address & listen & accept
  "log"
  "io" // for read/writer & copy
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
    go g.serve(conn)
  }
  return lfd.Close()
}

func (g *Melon) serve(conn net.Conn) error {
  defer conn.Close()

  paddr, err := net.ResolveTCPAddr("tcp", g.Proxy)
  if err != nil {
    log.Println(err)
  }
  if paddr != nil {
    pconn, err := net.DialTCP("tcp", nil, paddr)
    if err != nil {
      return err
    }
    return g.forward(conn, pconn)
  }

  daddr, err := net.ResolveTCPAddr("tcp", g.Daddr)
  if err != nil {
    log.Println(err)
  }
  if daddr != nil {
    dconn, err := net.DialTCP("tcp", nil, daddr)
    if err != nil {
      return err
    }
    defer dconn.Close()
    return g.transport(conn, dconn)
  }

  return nil
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
