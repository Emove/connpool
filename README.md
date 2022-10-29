# connpool

[![GoDoc][1]][2] [![Go Report Card][3]][4]

<!--[![Downloads][7]][8]-->

[1]: https://godoc.org/github.com/emove/connpool?status.svg

[2]: https://godoc.org/github.com/emove/connpool

[3]: https://goreportcard.com/badge/github.com/emove/connpool

[4]: https://goreportcard.com/report/github.com/emove/connpool

## 介绍
一个面向所有可复用链接的连接池。

## 特性
- 提供连接池最基本的功能，包括核心连接数，最大连接数，链接最大空闲时间，等待超时等
- 提供连接重试功能，包括定时重试，指数退避重试
- 允许在连接失败超过一定次数或时间后剔除连接地址
- 允许动态注入连接地址
- 提供WarmUp功能

## 安装
```shell
go get github.com/emove/connpool
```

## 示例
### 快速开始
```go
import (
    "fmt"
    "net"
    
    "github.com/emove/connpool"
)

func main() {
    // 创建一个连接池，传入三个基本参数
    // 连接地址，创建链接的方法，关闭链接的方法
    pool := connpool.NewPool([]string{addr}, dial, close,
        connpool.CoreNums(5), // 配置单个地址链接的核心连接数，默认配置为1
        connpool.MaxNums(20), // 配置单个地址链接的最大连接数，默认配置为10
        connpool.MaxIdleTime(10*time.Second), // 配置链接的最大空闲时间，默认配置为30秒
        connpool.WaitTimeout(5*time.Second), // 配置获取链接的超时时间，默认配置为3秒
    )
    con, err := pool.Get()
    if err != nil {
        fmt.Printf("get connection err: %v\n", err)
        return
    }
    
    conn := con.(net.Conn)
    _, err = conn.Write([]byte("hello world"))
    if err != nil {
        // 使用中发生错误，交由连接池销毁该连接
        pool.Discard(con)
    } else {
        // 将可用链接归还给连接池，等待复用
        pool.Put(con)
    }
    
    // 关闭连接池
    pool.Close()
}


// 定义连接函数，创建并返回一个tcp连接
func dial(addr string) (interface{}, error) {
    return net.Dial("tcp", addr)
}

// 定义连接关闭函数
func close(con interface{}) {
    if c, ok := con.(net.Conn); ok {
    	_ = c.Close()
    }
}
```

### 更多用法
[connpool-example](https://github.com/Emove/connpool/tree/main/example)