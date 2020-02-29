package main

import (
  "flag" // for main argv parse
  "log"
)

var melon Melon

func init() {
  flag.StringVar(&melon.Laddr, "L", ":8080", "listen address")
  flag.StringVar(&melon.Proxy, "P", "", "proxy for forward")
  flag.StringVar(&melon.Daddr, "S", "", "the server that connectiong to")
  flag.BoolVar(&melon.Shadows, "ss", false, "shadowsocks compatible")
  flag.BoolVar(&Debug, "d", false, "debug on")
  flag.Parse()

  log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
  log.Fatal(melon.Run())
}
