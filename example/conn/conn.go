package main

import (
	"context"
	"net"

	"github.com/emove/connpool"
)

var _ connpool.Connection = (*Conn)(nil)

// Conn 自定义链接数据结构，并实现connpool.Connection接口
type Conn struct {
	ctx context.Context
	raw net.Conn
}

// WithValue implements connpool.Connection#WithValue
func (c *Conn) WithValue(k, v interface{}) {
	c.ctx = context.WithValue(c.ctx, k, v)
}

// Value implements connpool.Connection#Value
func (c *Conn) Value(k interface{}) (v interface{}) {
	return c.ctx.Value(k)
}
