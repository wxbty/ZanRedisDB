package server

import (
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/siddontang/goredis"
	"github.com/stretchr/testify/assert"
	"github.com/youzan/ZanRedisDB/common"
	"github.com/youzan/ZanRedisDB/node"
	"github.com/youzan/ZanRedisDB/rockredis"
)

func TestKV(t *testing.T) {
	c := getTestConn(t)
	defer c.Close()

	key1 := "default:test:a"
	key2 := "default:test:b"
	keyExpire := "default:test:xx"

	if v, err := goredis.String(c.Do("getset", key1, "12345")); err != goredis.ErrNil {
		t.Logf("getset %v", v)
		t.Fatal(err)
	} else if v != "" {
		t.Fatal(v)
	}

	if ok, err := goredis.String(c.Do("set", key1, "1234")); err != nil {
		t.Fatal(err)
	} else if ok != OK {
		t.Fatal(ok)
	}

	if n, err := goredis.Int(c.Do("setnx", key1, "123")); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatal(n)
	}

	if n, err := goredis.Int(c.Do("setnx", key2, "123")); err != nil {
		t.Fatal(err)
	} else if n != 1 {
		t.Fatal(n)
	}

	if ok, err := goredis.String(c.Do("setex", keyExpire, 1, "hello world")); err != nil {
		t.Fatal(err)
	} else if ok != OK {
		t.Fatal(ok)
	}
	if v, err := goredis.String(c.Do("get", keyExpire)); err != nil {
		t.Fatal(err)
	} else if v != "hello world" {
		t.Fatal(v)
	}

	time.Sleep(time.Second * 4)
	if v, err := goredis.String(c.Do("get", keyExpire)); err != goredis.ErrNil {
		if err == nil && v == "hello world" {
			time.Sleep(time.Second * 16)
			if v, err := goredis.String(c.Do("get", keyExpire)); err != goredis.ErrNil {
				t.Fatalf("expired key should be expired: %v, %v", v, err)
			}
		} else {
			t.Fatalf("get expired key error: %v, %v", v, err)
		}
	}

	if v, err := goredis.String(c.Do("get", key1)); err != nil {
		t.Fatal(err)
	} else if v != "1234" {
		t.Fatal(v)
	}

	if v, err := goredis.String(c.Do("getset", key1, "123")); err != nil {
		t.Fatal(err)
	} else if v != "1234" {
		t.Fatal(v)
	}

	if v, err := goredis.String(c.Do("get", key1)); err != nil {
		t.Fatal(err)
	} else if v != "123" {
		t.Fatal(v)
	}

	if n, err := goredis.Int(c.Do("exists", key1)); err != nil {
		t.Fatal(err)
	} else if n != 1 {
		t.Fatal(n)
	}
	if n, err := goredis.Int(c.Do("exists", key1, key2)); err != nil {
		t.Fatal(err)
	} else if n != 2 {
		t.Fatal(n)
	}

	if n, err := goredis.Int(c.Do("exists", "default:test:empty_key_test")); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatal(n)
	}

	if _, err := goredis.Int(c.Do("del", key1, key2)); err != nil {
		t.Fatal(err)
	}

	if n, err := goredis.Int(c.Do("exists", key1)); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatal(n)
	}

	if n, err := goredis.Int(c.Do("exists", key2)); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatal(n)
	}

	if n, err := goredis.Int(c.Do("exists", key1, key2)); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatal(n)
	}
}

func TestKVPipeline(t *testing.T) {
	c := getTestConn(t)
	defer c.Close()
	pkey1 := "default:test:kvpla"
	pkey2 := "default:test:kvplb"

	err := c.Send("set", pkey1, "1")
	assert.Nil(t, err)
	err = c.Send("set", pkey2, "2")
	assert.Nil(t, err)
	v, err := goredis.String(c.Receive())
	assert.Nil(t, err)
	assert.Equal(t, OK, v)
	v, err = goredis.String(c.Receive())
	assert.Nil(t, err)
	assert.Equal(t, OK, v)
	if v, err := goredis.String(c.Do("get", pkey1)); err != nil {
		t.Fatal(err)
	} else if v != "1" {
		t.Error(v)
	}
	if v, err := goredis.String(c.Do("get", pkey2)); err != nil {
		t.Fatal(err)
	} else if v != "2" {
		t.Error(v)
	}
}

func TestKVExpire(t *testing.T) {
	c := getTestConn(t)
	defer c.Close()

	key1 := "default:test:expa"
	ttl := 3

	if ok, err := goredis.String(c.Do("setex", key1, ttl, "hello")); err != nil {
		t.Fatal(err)
	} else if ok != OK {
		t.Fatal(ok)
	}
	if v, err := goredis.String(c.Do("get", key1)); err != nil {
		t.Fatal(err)
	} else if v != "hello" {
		t.Fatal(v)
	}
	realTtl, err := goredis.Int(c.Do("ttl", key1))
	assert.Nil(t, err)
	assertTTLNear(t, ttl, realTtl)
	// check incr, append, setrange keep ttl
	if cnt, err := goredis.Int(c.Do("append", key1, " world")); err != nil {
		t.Fatal(err)
	} else if cnt != len("hello world") {
		t.Fatal(cnt)
	}
	if v, err := goredis.String(c.Do("get", key1)); err != nil {
		t.Fatal(err)
	} else if v != "hello world" {
		t.Fatal(v)
	}
	realTtl, err = goredis.Int(c.Do("ttl", key1))
	assert.Nil(t, err)
	assertTTLNear(t, ttl, realTtl)
	if cnt, err := goredis.Int(c.Do("setrange", key1, 1, "range")); err != nil {
		t.Fatal(err)
	} else if cnt != len("hrangeworld") {
		t.Fatal(cnt)
	}
	if v, err := goredis.String(c.Do("get", key1)); err != nil {
		t.Fatal(err)
	} else if v != "hrangeworld" {
		t.Fatal(v)
	}
	realTtl, err = goredis.Int(c.Do("ttl", key1))
	assert.Nil(t, err)
	assertTTLNear(t, ttl, realTtl)

	time.Sleep(time.Second * time.Duration(ttl+2))
	if v, err := goredis.String(c.Do("get", key1)); err != goredis.ErrNil {
		t.Fatalf("expired key should be expired: %v, %v", v, err)
	}

	realTtl, err = goredis.Int(c.Do("ttl", key1))
	assert.Nil(t, err)
	assert.Equal(t, -1, realTtl)

	if ok, err := goredis.String(c.Do("setex", key1, ttl, "1")); err != nil {
		t.Fatal(err)
	} else if ok != OK {
		t.Fatal(ok)
	}
	if v, err := goredis.String(c.Do("get", key1)); err != nil {
		t.Fatal(err)
	} else if v != "1" {
		t.Fatal(v)
	}
	realTtl, err = goredis.Int(c.Do("ttl", key1))
	assert.Nil(t, err)
	assertTTLNear(t, ttl, realTtl)
	_, err = c.Do("incr", key1)
	assert.Nil(t, err)
	if v, err := goredis.String(c.Do("get", key1)); err != nil {
		t.Fatal(err)
	} else if v != "2" {
		t.Fatal(v)
	}
	realTtl, err = goredis.Int(c.Do("ttl", key1))
	assert.Nil(t, err)
	assertTTLNear(t, ttl, realTtl)
	//persist
	c.Do("persist", key1)
	realTtl, err = goredis.Int(c.Do("ttl", key1))
	assert.Nil(t, err)
	assert.Equal(t, -1, realTtl)

	// change ttl
	_, err = c.Do("expire", key1, ttl+4)
	assert.Nil(t, err)
	realTtl, err = goredis.Int(c.Do("ttl", key1))
	assert.Nil(t, err)
	assertTTLNear(t, ttl+4, realTtl)

	time.Sleep(time.Second * time.Duration(ttl+5))
	// check expired kv should not get from any read command
	if v, err := goredis.String(c.Do("get", key1)); err != goredis.ErrNil {
		t.Fatalf("expired key should be expired: %v, %v", v, err)
	}
	realTtl, err = goredis.Int(c.Do("ttl", key1))
	assert.Nil(t, err)
	assert.Equal(t, -1, realTtl)
	if n, err := goredis.Int(c.Do("exists", key1)); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatal(n)
	}
	vv, err := goredis.MultiBulk(c.Do("mget", key1))
	assert.Nil(t, err)
	assert.Equal(t, nil, vv[0])
	_, err = goredis.String(c.Do("getset", key1, "new1"))
	assert.Equal(t, goredis.ErrNil, err)
	nv, err := goredis.String(c.Do("get", key1))
	assert.Nil(t, err)
	assert.Equal(t, "new1", nv)

	// persist
	if ok, err := goredis.String(c.Do("setex", key1, ttl, "1")); err != nil {
		t.Fatal(err)
	} else if ok != OK {
		t.Fatal(ok)
	}
	realTtl, err = goredis.Int(c.Do("ttl", key1))
	assert.Nil(t, err)
	assertTTLNear(t, ttl, realTtl)

	_, err = c.Do("persist", key1)
	assert.Nil(t, err)

	realTtl, err = goredis.Int(c.Do("ttl", key1))
	assert.Nil(t, err)
	assert.Equal(t, -1, realTtl)
}

func TestKVM(t *testing.T) {
	c := getTestConn(t)
	defer c.Close()

	key1 := "default:test:kvma"
	key2 := "default:test:kvmb"
	key3 := "default:test:kvmc"
	if ok, err := goredis.String(c.Do("set", key1, "1")); err != nil {
		t.Fatal(err)
	} else if ok != OK {
		t.Fatal(ok)
	}
	if ok, err := goredis.String(c.Do("set", key2, "2")); err != nil {
		t.Fatal(err)
	} else if ok != OK {
		t.Fatal(ok)
	}

	if v, err := goredis.String(c.Do("get", key1)); err != nil {
		t.Fatal(err)
	} else if v != "1" {
		t.Error(v)
	}
	if v, err := goredis.String(c.Do("get", key2)); err != nil {
		t.Fatal(err)
	} else if v != "2" {
		t.Error(v)
	}

	if v, err := goredis.MultiBulk(c.Do("mget", key1, key2, key3)); err != nil {
		t.Fatal(err)
	} else if len(v) != 3 {
		t.Fatal(len(v))
	} else {
		if vv, ok := v[0].([]byte); !ok || string(vv) != "1" {
			t.Fatalf("not 1, %v", v)
		}

		if vv, ok := v[1].([]byte); !ok || string(vv) != "2" {
			t.Errorf("not 2, %v", v[1])
		}

		if v[2] != nil {
			t.Errorf("must nil: %v", v[2])
		}
	}
}

func TestKVIncrDecr(t *testing.T) {
	c := getTestConn(t)
	defer c.Close()

	key := "default:test:kv_n"
	if n, err := goredis.Int64(c.Do("incr", key)); err != nil {
		t.Fatal(err)
	} else if n != 1 {
		t.Fatal(n)
	}

	if n, err := goredis.Int64(c.Do("incr", key)); err != nil {
		t.Fatal(err)
	} else if n != 2 {
		t.Fatal(n)
	}

	if n, err := goredis.Int64(c.Do("incrby", key, 10)); err != nil {
		t.Fatal(err)
	} else if n != 12 {
		t.Fatal(n)
	}

	if n, err := goredis.Int64(c.Do("incrby", key, -10)); err != nil {
		t.Fatal(err)
	} else if n != 2 {
		t.Fatal(n)
	}
}

func TestKVBitOp(t *testing.T) {
	c := getTestConn(t)
	defer c.Close()

	key := "default:test:kv_bitop"
	if n, err := goredis.Int64(c.Do("bitcount", key)); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatal(n)
	}

	if n, err := goredis.Int64(c.Do("getbit", key, 1)); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatal(n)
	}

	if n, err := goredis.Int64(c.Do("setbit", key, 100, 1)); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatal(n)
	}

	if n, err := goredis.Int64(c.Do("getbit", key, 100)); err != nil {
		t.Fatal(err)
	} else if n != 1 {
		t.Fatal(n)
	}
	if n, err := goredis.Int64(c.Do("bitcount", key)); err != nil {
		t.Fatal(err)
	} else if n != 1 {
		t.Fatal(n)
	}
	if n, err := goredis.Int64(c.Do("setbit", key, 1, 1)); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatal(n)
	}
	if n, err := goredis.Int64(c.Do("bitcount", key)); err != nil {
		t.Fatal(err)
	} else if n != 2 {
		t.Fatal(n)
	}
	if n, err := goredis.Int64(c.Do("bitcount", key, 0, 0)); err != nil {
		t.Fatal(err)
	} else if n != 1 {
		t.Fatal(n)
	}
	if n, err := goredis.Int64(c.Do("setbit", key, 8, 1)); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatal(n)
	}
	if n, err := goredis.Int64(c.Do("bitcount", key, 0, 0)); err != nil {
		t.Fatal(err)
	} else if n != 1 {
		t.Fatal(n)
	}
	if n, err := goredis.Int64(c.Do("setbit", key, 7, 1)); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatal(n)
	}
	if n, err := goredis.Int64(c.Do("bitcount", key, 0, 0)); err != nil {
		t.Fatal(err)
	} else if n != 2 {
		t.Fatal(n)
	}
	if n, err := goredis.Int64(c.Do("bitcount", key, 0, 1)); err != nil {
		t.Fatal(err)
	} else if n != 3 {
		t.Fatal(n)
	}
	_, err := goredis.Int64(c.Do("setbit", key, -7, 1))
	assert.NotNil(t, err)
}

func TestKVBitExpire(t *testing.T) {
	c := getTestConn(t)
	defer c.Close()

	key1 := "default:test:bit_exp"
	ttl := 3

	if n, err := goredis.Int(c.Do("setbitv2", key1, 1, 1)); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatal(n)
	}
	if v, err := goredis.Int(c.Do("getbit", key1, 1)); err != nil {
		t.Fatal(err)
	} else if v != 1 {
		t.Fatal(v)
	}
	c.Do("bexpire", key1, ttl)
	realTtl, err := goredis.Int(c.Do("bttl", key1))
	assert.Nil(t, err)
	assertTTLNear(t, ttl, realTtl)
	// check write keep ttl
	if _, err := goredis.Int(c.Do("setbitv2", key1, 2, 1)); err != nil {
		t.Fatal(err)
	}
	if v, err := goredis.Int(c.Do("getbit", key1, 2)); err != nil {
		t.Fatal(err)
	} else if v != 1 {
		t.Fatal(v)
	}
	realTtl, err = goredis.Int(c.Do("bttl", key1))
	assert.Nil(t, err)
	assertTTLNear(t, ttl, realTtl)

	time.Sleep(time.Second * time.Duration(ttl+2))
	if v, err := goredis.Int(c.Do("getbit", key1, 1)); err != goredis.ErrNil && err != nil {
		t.Fatalf("expired key should be expired: %v, %v", v, err)
	} else if v != 0 {
		t.Fatal(v)
	}

	realTtl, err = goredis.Int(c.Do("bttl", key1))
	assert.Nil(t, err)
	assert.Equal(t, -1, realTtl)

	if n, err := goredis.Int(c.Do("setbitv2", key1, 3, 1)); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatal(n)
	}
	if n, err := goredis.Int(c.Do("bexpire", key1, ttl)); err != nil {
		t.Fatal(err)
	} else if n != 1 {
		t.Fatal(n)
	}
	if v, err := goredis.Int(c.Do("getbit", key1, 3)); err != nil {
		t.Fatal(err)
	} else if v != 1 {
		t.Fatal(v)
	}
	if v, err := goredis.Int(c.Do("getbit", key1, 1)); err != nil {
		t.Fatal(err)
	} else if v != 0 {
		t.Fatal(v)
	}
	realTtl, err = goredis.Int(c.Do("bttl", key1))
	assert.Nil(t, err)
	assertTTLNear(t, ttl, realTtl)
	if n, err := goredis.Int(c.Do("setbitv2", key1, 4, 1)); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatal(n)
	}
	realTtl, err = goredis.Int(c.Do("bttl", key1))
	assert.Nil(t, err)
	assertTTLNear(t, ttl, realTtl)
	//persist
	c.Do("bpersist", key1)
	realTtl, err = goredis.Int(c.Do("bttl", key1))
	assert.Nil(t, err)
	assert.Equal(t, -1, realTtl)

	// change ttl
	_, err = c.Do("bexpire", key1, ttl+4)
	assert.Nil(t, err)
	realTtl, err = goredis.Int(c.Do("bttl", key1))
	assert.Nil(t, err)
	assertTTLNear(t, ttl+4, realTtl)

	time.Sleep(time.Second * time.Duration(ttl+5))
	// check expired kv should not get from any read command
	if v, err := goredis.Int(c.Do("getbit", key1, 3)); err != goredis.ErrNil && err != nil {
		t.Fatalf("expired key should be expired: %v, %v", v, err)
	} else if v != 0 {
		t.Fatal(v)
	}
	realTtl, err = goredis.Int(c.Do("bttl", key1))
	assert.Nil(t, err)
	assert.Equal(t, -1, realTtl)
	if n, err := goredis.Int(c.Do("bkeyexist", key1)); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatal(n)
	}

	// persist
	if n, err := goredis.Int(c.Do("setbitv2", key1, 5, 1)); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatal(n)
	}

	_, err = c.Do("bexpire", key1, ttl)
	assert.Nil(t, err)

	realTtl, err = goredis.Int(c.Do("bttl", key1))
	assert.Nil(t, err)
	assertTTLNear(t, ttl, realTtl)

	_, err = c.Do("bpersist", key1)
	assert.Nil(t, err)

	realTtl, err = goredis.Int(c.Do("bttl", key1))
	assert.Nil(t, err)
	assert.Equal(t, -1, realTtl)
}

func TestKVBatch(t *testing.T) {

	var wg sync.WaitGroup
	concurrency := 100
	poolList := make([]*goredis.PoolConn, concurrency)
	for i := 0; i < concurrency; i++ {
		poolList[i] = getTestConn(t)
	}
	defer func() {
		for i := 0; i < concurrency; i++ {
			poolList[i].Close()
		}
	}()
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(index int, c *goredis.PoolConn) {
			defer wg.Done()

			key1 := "default:test:a" + strconv.Itoa(index)
			key2 := "default:test:b" + strconv.Itoa(index)
			key3 := "default:test:c" + strconv.Itoa(index)
			key4 := "default:test:d" + strconv.Itoa(index)
			keyExpire := "default:test:xx" + strconv.Itoa(index)
			if ok, err := goredis.String(c.Do("set", key1, "1234")); err != nil {
				t.Fatal(err)
			} else if ok != OK {
				t.Fatal(ok)
			}

			if n, err := goredis.Int(c.Do("setnx", key1, "123")); err != nil {
				t.Fatal(err)
			} else if n != 0 {
				t.Fatal(n)
			}

			if n, err := goredis.Int(c.Do("setnx", key2, "123")); err != nil {
				t.Fatal(err)
			} else if n != 1 {
				t.Fatal(n)
			}

			if ok, err := goredis.String(c.Do("set", key3, key3)); err != nil {
				t.Fatal(err)
			} else if ok != OK {
				t.Fatal(ok)
			}
			if v, err := goredis.String(c.Do("get", key3)); err != nil {
				t.Fatal(err)
			} else if v != key3 {
				t.Fatal(v)
			}

			if ok, err := goredis.String(c.Do("setex", keyExpire, 3, "hello world")); err != nil {
				t.Fatal(err)
			} else if ok != OK {
				t.Fatal(ok)
			}
			if v, err := goredis.String(c.Do("get", keyExpire)); err != nil {
				t.Fatal(err)
			} else if v != "hello world" {
				t.Fatal(v)
			}

			if ok, err := goredis.String(c.Do("set", key4, key4)); err != nil {
				t.Fatal(err)
			} else if ok != OK {
				t.Fatal(ok)
			}
			if v, err := goredis.String(c.Do("get", key4)); err != nil {
				t.Fatal(err)
			} else if v != key4 {
				t.Fatal(v)
			}

			mkey1 := "default:test:kvma" + strconv.Itoa(index)
			mkey2 := "default:test:kvmb" + strconv.Itoa(index)
			mkey3 := "default:test:kvmc" + strconv.Itoa(index)
			// test pipeline set
			err := c.Send("set", mkey1, "1")
			assert.Nil(t, err)
			err = c.Send("set", mkey2, "2")
			assert.Nil(t, err)
			v, err := goredis.String(c.Receive())
			assert.Nil(t, err)
			assert.Equal(t, OK, v)
			v, err = goredis.String(c.Receive())
			assert.Nil(t, err)
			assert.Equal(t, OK, v)

			if v, err := goredis.String(c.Do("get", mkey1)); err != nil {
				t.Fatal(err)
			} else if v != "1" {
				t.Error(v)
			}
			if v, err := goredis.String(c.Do("get", mkey2)); err != nil {
				t.Fatal(err)
			} else if v != "2" {
				t.Error(v)
			}

			if v, err := goredis.MultiBulk(c.Do("mget", mkey1, mkey2, mkey3)); err != nil {
				t.Fatal(err)
			} else if len(v) != 3 {
				t.Fatal(len(v))
			} else {
				if vv, ok := v[0].([]byte); !ok || string(vv) != "1" {
					t.Fatalf("not 1, %v", v)
				}

				if vv, ok := v[1].([]byte); !ok || string(vv) != "2" {
					t.Errorf("not 2, %v", v[1])
				}

				if v[2] != nil {
					t.Errorf("must nil: %v", v[2])
				}
			}

			time.Sleep(time.Second * 4)
			if v, err := goredis.String(c.Do("get", keyExpire)); err != goredis.ErrNil {
				if err == nil && v == "hello world" {
					time.Sleep(time.Second * 16)
					if v, err := goredis.String(c.Do("get", keyExpire)); err != goredis.ErrNil {
						t.Fatalf("expired key should be expired: %v, %v", v, err)
					}
				} else {
					t.Fatalf("get expired key error: %v, %v", v, err)
				}
			}

			if v, err := goredis.String(c.Do("get", key1)); err != nil {
				t.Fatal(err)
			} else if v != "1234" {
				t.Fatal(v)
			}

			if n, err := goredis.Int(c.Do("exists", key1)); err != nil {
				t.Fatal(err)
			} else if n != 1 {
				t.Fatal(n)
			}

			if n, err := goredis.Int(c.Do("exists", "default:test:empty_key_test"+strconv.Itoa(index))); err != nil {
				t.Fatal(err)
			} else if n != 0 {
				t.Fatal(n)
			}

			if _, err := goredis.Int(c.Do("del", key1, key2)); err != nil {
				t.Fatal(err)
			}

			if n, err := goredis.Int(c.Do("exists", key1)); err != nil {
				t.Fatal(err)
			} else if n != 0 {
				t.Fatal(n)
			}

			if n, err := goredis.Int(c.Do("exists", key2)); err != nil {
				t.Fatal(err)
			} else if n != 0 {
				t.Fatal(n)
			}
		}(i, poolList[i])
	}
	wg.Wait()

}

func TestKVStringOp(t *testing.T) {
	c := getTestConn(t)
	defer c.Close()

	key := "default:test:kv_stringop"
	if n, err := goredis.Int64(c.Do("strlen", key)); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatal(n)
	}
	c.Do("setex", key, 10, "Hello")
	n, err := goredis.Int64(c.Do("strlen", key))
	assert.Nil(t, err)
	assert.Equal(t, len("Hello"), int(n))
	// append
}

func TestKVErrorParams(t *testing.T) {
	c := getTestConn(t)
	defer c.Close()

	key1 := "default:test:kv_erra"
	key2 := "default:test:kv_errb"
	key3 := "default:test:kv_errc"
	_, err := c.Do("get", key1, key2, key3)
	assert.NotNil(t, err)

	_, err = c.Do("set", key1, key2, key3)
	assert.NotNil(t, err)

	_, err = c.Do("getset", key1, key2, key3)
	assert.NotNil(t, err)

	_, err = c.Do("setnx", key1, key2, key3)
	assert.NotNil(t, err)

	_, err = c.Do("exists")
	assert.NotNil(t, err)

	_, err = c.Do("incr", key1, key2)
	assert.NotNil(t, err)

	_, err = c.Do("incrby", key1)
	assert.NotNil(t, err)

	_, err = c.Do("incrby", key1, "nan")
	assert.NotNil(t, err)

	_, err = c.Do("decrby", key1)
	assert.NotNil(t, err)

	_, err = c.Do("del")
	assert.NotNil(t, err)

	_, err = c.Do("mset")
	assert.NotNil(t, err)

	_, err = c.Do("mset", key1, key2, key3)
	assert.NotNil(t, err)

	_, err = c.Do("mget")
	assert.NotNil(t, err)

	_, err = c.Do("getbit")
	assert.NotNil(t, err)

	_, err = c.Do("getbit", key1)
	assert.NotNil(t, err)

	_, err = c.Do("setbit", key1)
	assert.NotNil(t, err)

	_, err = c.Do("setbit")
	assert.NotNil(t, err)

	_, err = c.Do("bitcount")
	assert.NotNil(t, err)

	_, err = c.Do("bitcount", key1, "0")
	assert.NotNil(t, err)
}

func TestPFOp(t *testing.T) {
	if testing.Verbose() {
		rockredis.SetLogger(int32(common.LOG_DETAIL), newTestLogger(t))
	}
	c := getTestConn(t)
	defer c.Close()

	key1 := "default:test:pf_a"
	cnt, err := goredis.Int64(c.Do("pfcount", key1))
	assert.Nil(t, err)
	assert.Equal(t, int64(0), cnt)

	// first init with no element
	cnt, err = goredis.Int64(c.Do("pfadd", key1))
	assert.Nil(t, err)
	assert.Equal(t, int64(1), cnt)

	cnt, err = goredis.Int64(c.Do("pfadd", key1, 1))
	assert.Nil(t, err)
	assert.Equal(t, int64(1), cnt)

	cnt, err = goredis.Int64(c.Do("pfcount", key1))
	assert.Nil(t, err)
	assert.Equal(t, int64(1), cnt)

	cnt, err = goredis.Int64(c.Do("pfadd", key1, 1))
	assert.Nil(t, err)

	cnt, err = goredis.Int64(c.Do("pfcount", key1))
	assert.Nil(t, err)
	assert.Equal(t, int64(1), cnt)

	// test pfadd with no element on exist key
	cnt, err = goredis.Int64(c.Do("pfadd", key1))
	assert.Nil(t, err)
	assert.Equal(t, int64(0), cnt)

	cnt, err = goredis.Int64(c.Do("pfadd", key1, 1, 2, 3))
	assert.Nil(t, err)
	assert.Equal(t, int64(1), cnt)

	cnt, err = goredis.Int64(c.Do("pfcount", key1))
	assert.Nil(t, err)
	assert.Equal(t, int64(3), cnt)

	c.Do("del", key1)

	cnt, err = goredis.Int64(c.Do("pfcount", key1))
	assert.Nil(t, err)
	assert.Equal(t, int64(0), cnt)
}

func TestPFOpErrorParams(t *testing.T) {
	c := getTestConn(t)
	defer c.Close()

	key1 := "default:test:pf_erra"
	key2 := "default:test:pf_errb"
	_, err := c.Do("pfadd")
	assert.NotNil(t, err)

	_, err = c.Do("pfcount", key1, key2)
	assert.NotNil(t, err)

	_, err = c.Do("pfcount")
	assert.NotNil(t, err)
}

func TestSyncerOnlyWrite(t *testing.T) {
	c := getTestConn(t)
	defer c.Close()

	key1 := "default:test:synceronly"
	key2 := "default:test:synceronly2"
	_, err := goredis.String(c.Do("set", key1, "1234"))
	_, err = goredis.String(c.Do("set", key2, "1234"))
	assert.Nil(t, err)
	node.SetSyncerOnly(true)
	defer node.SetSyncerOnly(false)

	_, err = goredis.String(c.Do("getset", key1, "12345"))
	assert.NotNil(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "The cluster is only allowing syncer write"))
	_, err = goredis.String(c.Do("set", key1, "12345"))
	assert.NotNil(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "The cluster is only allowing syncer write"))
	_, err = goredis.String(c.Do("plset", key1, "12345"))
	assert.NotNil(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "The cluster is only allowing syncer write"))

	// failed write should not change the key value
	if v, err := goredis.String(c.Do("get", key1)); err != nil {
		t.Fatal(err)
	} else if v != "1234" {
		t.Fatal(v)
	}

	if ay, err := goredis.Values(c.Do("ADVSCAN", "default:testscan:"+"", "kv", "count", 5)); err != nil {
		t.Error(err)
	} else if len(ay) != 2 {
		t.Fatal(len(ay))
	}

	if ay, err := goredis.Values(c.Do("SCAN", "default:testscan:"+"", "count", 5)); err != nil {
		t.Error(err)
	} else if len(ay) != 2 {
		t.Fatal(len(ay))
	}

	if n, err := goredis.Int(c.Do("exists", key1)); err != nil {
		t.Fatal(err)
	} else if n != 1 {
		t.Fatal(n)
	}
	if n, err := goredis.Int(c.Do("exists", key1, key2)); err != nil {
		t.Fatal(err)
	} else if n != 2 {
		t.Fatal(n)
	}

	_, err = goredis.Int(c.Do("del", key1, key2))
	assert.NotNil(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "The cluster is only allowing syncer write"))

	// failed del should not change the key
	if n, err := goredis.Int(c.Do("exists", key1)); err != nil {
		t.Fatal(err)
	} else if n != 1 {
		t.Fatal(n)
	}
	if n, err := goredis.Int(c.Do("exists", key1, key2)); err != nil {
		t.Fatal(err)
	} else if n != 2 {
		t.Fatal(n)
	}
}
