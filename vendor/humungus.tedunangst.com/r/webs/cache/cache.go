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

// A simple in memory, in process cache
package cache

import (
	"errors"
	"reflect"
	"sync"
	"time"

	"humungus.tedunangst.com/r/webs/gate"
)

// Fill functions should be roughtly compatible with this type.
// They may use stronger types, however.
// It will be called after a cache miss.
// It should return a value and bool indicating success.
type Filler func(key interface{}) (interface{}, bool)

// Arguments to creating a new cache.
// Filler is required. See Filler type documentation.
// The cache will consider itself stale after Duration passes from
// the first fill.
// Invalidator allows invalidating multiple dependent caches.
// Limit is max entries, fifo fashion.
type Options struct {
	Filler      interface{}
	Duration    time.Duration
	Invalidator *Invalidator
	Limit       int
}

// The cache object
type Cache struct {
	cache      map[interface{}]interface{}
	filler     Filler
	lock       sync.Mutex
	stale      time.Time
	duration   time.Duration
	serializer *gate.Serializer
	serialno int
	fifo       []interface{}
	fifopos    int
}

// An Invalidator is a collection of caches to be cleared or flushed together.
// It is created, then its address passed to cache creation.
type Invalidator struct {
	caches []*Cache
}

// Create a new Cache. Arguments are provided via Options.
func New(options Options) *Cache {
	c := new(Cache)
	c.cache = make(map[interface{}]interface{})
	fillfn := options.Filler
	if fillfn != nil {
		ftype := reflect.TypeOf(fillfn)
		if ftype.Kind() != reflect.Func {
			panic("cache filler is not function")
		}
		if ftype.NumIn() != 1 || ftype.NumOut() != 2 {
			panic("cache filler has wrong argument count")
		}
		c.filler = func(key interface{}) (interface{}, bool) {
			vfn := reflect.ValueOf(fillfn)
			args := []reflect.Value{reflect.ValueOf(key)}
			rv := vfn.Call(args)
			return rv[0].Interface(), rv[1].Bool()
		}
	}
	if options.Duration != 0 {
		c.duration = options.Duration
		c.stale = time.Now().Add(c.duration)
	}
	if options.Invalidator != nil {
		options.Invalidator.caches = append(options.Invalidator.caches, c)
	}
	c.serializer = gate.NewSerializer()
	if options.Limit != 0 {
		c.fifo = make([]interface{}, options.Limit)
	}
	return c
}

// Get a value for a key. Returns true for success.
// Will automatically fill the cache.
// Returns holding the cache lock. Useful when the cached value can mutate.
func (cache *Cache) GetAndLock(key interface{}, value interface{}) bool {
	cache.lock.Lock()
	if !cache.stale.IsZero() && cache.stale.Before(time.Now()) {
		cache.stale = time.Now().Add(cache.duration)
		cache.cache = make(map[interface{}]interface{})
	}
recheck:
	v, ok := cache.cache[key]
	if !ok {
		if cache.filler == nil {
			return false
		}
		serial := cache.serialno
		cache.lock.Unlock()
		r, err := cache.serializer.Call(key, func() (interface{}, error) {
			v, ok := cache.filler(key)
			if !ok {
				return nil, errors.New("no fill")
			}
			return v, nil
		})
		cache.lock.Lock()
		if err == gate.Cancelled || serial != cache.serialno {
			goto recheck
		}
		if err == nil {
			v, ok = r, true
		}
		if ok {
			cache.set(key, v)
		}
	}
	if ok {
		ptr := reflect.ValueOf(v)
		reflect.ValueOf(value).Elem().Set(ptr)
	}
	return ok
}

// Get a value for a key. Returns true for success.
// Will automatically fill the cache.
func (cache *Cache) Get(key interface{}, value interface{}) bool {
	rv := cache.GetAndLock(key, value)
	cache.lock.Unlock()
	return rv
}

func (cache *Cache) set(key interface{}, value interface{}) {
	cache.cache[key] = value
	if cache.fifo != nil {
		pos := cache.fifopos + 1
		if pos == len(cache.fifo) {
			pos = 0
		}
		delete(cache.cache, cache.fifo[pos])
		cache.fifo[pos] = key
		cache.fifopos = pos
	}
}

// Manually set a cached value.
func (cache *Cache) Set(key interface{}, value interface{}) {
	cache.lock.Lock()
	cache.set(key, value)
	cache.lock.Unlock()
}

// Unlock the cache, iff lock is held.
func (cache *Cache) Unlock() {
	cache.lock.Unlock()
}

// Clear one key from the cache
func (cache *Cache) Clear(key interface{}) {
	cache.lock.Lock()
	cache.serialno++
	delete(cache.cache, key)
	for i, k := range cache.fifo {
		if k == key {
			cache.fifo[i] = nil
			break
		}
	}
	cache.serializer.Cancel(key)
	cache.lock.Unlock()
}

// Flush all values from the cache
func (cache *Cache) Flush() {
	cache.lock.Lock()
	cache.serialno++
	cache.cache = make(map[interface{}]interface{})
	for i, _ := range cache.fifo {
		cache.fifo[i] = nil
	}
	cache.serializer.CancelAll()
	cache.lock.Unlock()
}

// Clear one key from associated caches
func (inv Invalidator) Clear(key interface{}) {
	for _, c := range inv.caches {
		c.Clear(key)
	}
}

// Flush all values from associated caches
func (inv Invalidator) Flush() {
	for _, c := range inv.caches {
		c.Flush()
	}
}
