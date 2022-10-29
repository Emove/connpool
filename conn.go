package connpool

// Connection is used to attach some information to the connection to relieve the pressure of the pool.
// Implements it to gain a better performance when the pool working in a heavily load.
// For example:
// 		type Conn struct {
// 			ctx context.Context
// 		}
//
// 		func (c *Conn) Value(k interface{}) interface{} {
//			return c.ctx.Value(k)
//		}
//
//		func (c *Conn) WithValue(k, v interface{}) {
//			c.ctx = context.WithValue(c.ctx, k, v)
//		}
type Connection interface {
	Value(k interface{}) interface{}
	WithValue(k, v interface{})
}
