//
// Copyright (c) 2019 Ted Unangst <tedu@tedunangst.com>
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that the above
// copyright notice and this permission notice appear in all copies.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
// WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
// ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
// WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
// ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
// OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

// The gate package provides rate limiters and serializers.
package gate

import (
	"sync"
)

// Limiter limits the number of concurrent outstanding operations.
// Typical usage: limiter.Start(); defer limiter.Finish()
type Limiter struct {
	maxout int
	numout int
	lock   sync.Mutex
	bell   *sync.Cond
}

// Create a new Limiter with maxout operations
func NewLimiter(maxout int) *Limiter {
	l := new(Limiter)
	l.maxout = maxout
	l.bell = sync.NewCond(&l.lock)
	return l
}

// Wait for an opening, then return when ready.
func (l *Limiter) Start() {
	l.lock.Lock()
	for l.numout >= l.maxout {
		l.bell.Wait()
	}
	l.numout++
	l.lock.Unlock()
}

// Free an opening after finishing.
func (l *Limiter) Finish() {
	l.lock.Lock()
	l.numout--
	l.bell.Broadcast()
	l.lock.Unlock()
}

type result struct {
	res interface{}
	err error
}

// Serializer restricts function calls to one at a time per key.
// Saved results from the first call are returned.
// (To only download a resource a single time.)
type Serializer struct {
	gates map[interface{}][]chan result
	lock  sync.Mutex
}

// Create a new Serializer
func NewSerializer() *Serializer {
	g := new(Serializer)
	g.gates = make(map[interface{}][]chan result)
	return g
}

// Call fn, gated by key.
// Subsequent calls with the same key will wait until the first returns,
// then all functions return the same result.
func (g *Serializer) Call(key interface{}, fn func() (interface{}, error)) (interface{}, error) {
	g.lock.Lock()
	inflight, ok := g.gates[key]
	if ok {
		c := make(chan result)
		g.gates[key] = append(inflight, c)
		g.lock.Unlock()
		r := <-c
		close(c)
		return r.res, r.err
	}
	g.gates[key] = inflight
	g.lock.Unlock()

	res, err := fn()

	g.lock.Lock()
	inflight = g.gates[key]
	delete(g.gates, key)
	g.lock.Unlock()
	if len(inflight) > 0 {
		r := result{res: res, err: err}
		go func() {
			for _, c := range inflight {
				c <- r
			}
		}()
	}
	return res, err
}
