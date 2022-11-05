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

// A function which returns the size of an element in the cache.
// It may be stronger typed.
type Sizer func(res interface{}) int

// A function which reduces a complex key into one suitable for a map.
// It may be stronger typed.
type Reducer func(key interface{}) interface{}

// Arguments to creating a new cache.
// Filler is required. See Filler type documentation.
// Entries will expire after Duration if set.
// Invalidator allows invalidating multiple dependent caches.
// Limit is max entries.
// SizeLimit is max size of all elements, combined with Sizer.
// Reducer allows for the use of complex keys.
type Options struct {
	Filler      interface{}
	Duration    time.Duration
	Invalidator *Invalidator
	Limit       int
	SizeLimit   int
	Sizer       interface{}
	Reducer     interface{}
	Singleton   bool
}

type entry struct {
	value interface{}
	size  int
	stale time.Time
}

type entrymap map[interface{}]entry

// The cache object
type Cache struct {
	cache      entrymap
	filler     Filler
	sizer      Sizer
	reducer    Reducer
	lock       sync.Mutex
	duration   time.Duration
	serializer *gate.Serializer
	serialno   int
	limit      int
	size       int
	sizelimit  int
	singleton  bool
}

// An Invalidator is a collection of caches to be cleared or flushed together.
// It is created, then its address passed to cache creation.
type Invalidator struct {
	caches []*Cache
}

// Create a new Cache. Arguments are provided via Options.
func New(options Options) *Cache {
	c := new(Cache)
	c.cache = make(entrymap)
	if fillfn := options.Filler; fillfn != nil {
		ftype := reflect.TypeOf(fillfn)
		if ftype.Kind() != reflect.Func {
			panic("cache filler is not function")
		}
		if ftype.NumIn() != 1 || ftype.NumOut() != 2 {
			panic("cache filler has wrong argument count")
		}
		vfn := reflect.ValueOf(fillfn)
		c.filler = func(key interface{}) (interface{}, bool) {
			args := []reflect.Value{reflect.ValueOf(key)}
			rv := vfn.Call(args)
			return rv[0].Interface(), rv[1].Bool()
		}
	}
	if sizefn := options.Sizer; sizefn != nil {
		ftype := reflect.TypeOf(sizefn)
		if ftype.Kind() != reflect.Func {
			panic("cache sizer is not function")
		}
		if ftype.NumIn() != 1 || ftype.NumOut() != 1 {
			panic("cache sizer has wrong argument count")
		}
		vfn := reflect.ValueOf(sizefn)
		c.sizer = func(res interface{}) int {
			args := []reflect.Value{reflect.ValueOf(res)}
			rv := vfn.Call(args)
			return int(rv[0].Int())
		}
	}
	if reducefn := options.Reducer; reducefn != nil {
		ftype := reflect.TypeOf(reducefn)
		if ftype.Kind() != reflect.Func {
			panic("cache sizer is not function")
		}
		if ftype.NumIn() != 1 || ftype.NumOut() != 1 {
			panic("cache sizer has wrong argument count")
		}
		vfn := reflect.ValueOf(reducefn)
		c.reducer = func(res interface{}) interface{} {
			args := []reflect.Value{reflect.ValueOf(res)}
			rv := vfn.Call(args)
			return rv[0].Interface()
		}
	}
	if options.Duration != 0 {
		c.duration = options.Duration
	}
	if options.Invalidator != nil {
		options.Invalidator.caches = append(options.Invalidator.caches, c)
	}
	c.serializer = gate.NewSerializer()
	c.limit = options.Limit
	c.sizelimit = options.SizeLimit
	c.singleton = options.Singleton
	return c
}

// Get a value for a key. Returns true for success.
// Will automatically fill the cache.
// Returns holding the cache lock. Useful when the cached value can mutate.
func (c *Cache) GetAndLock(key interface{}, value interface{}) bool {
	origkey := key
	if c.reducer != nil {
		key = c.reducer(key)
	}
	c.lock.Lock()
recheck:
	ent, ok := c.cache[key]
	if ok {
		if !ent.stale.IsZero() && ent.stale.Before(time.Now()) {
			c.remove(key, ent)
			ok = false
		}
	}
	if !ok {
		if c.filler == nil {
			return false
		}
		serial := c.serialno
		c.lock.Unlock()
		r, err := c.serializer.Call(key, func() (interface{}, error) {
			v, ok := c.filler(origkey)
			if !ok {
				return nil, errors.New("no fill")
			}
			return v, nil
		})
		c.lock.Lock()
		if err == gate.Cancelled || serial != c.serialno {
			goto recheck
		}
		if err == nil {
			c.set(key, r)
			ent.value, ok = r, true
		}
	}
	if ok {
		ptr := reflect.ValueOf(ent.value)
		reflect.ValueOf(value).Elem().Set(ptr)
	}
	return ok
}

// Get a value for a key. Returns true for success.
// Will automatically fill the cache.
func (c *Cache) Get(key interface{}, value interface{}) bool {
	rv := c.GetAndLock(key, value)
	c.lock.Unlock()
	return rv
}

func (c *Cache) set(key interface{}, value interface{}) {
	var stale time.Time
	if c.duration != 0 {
		stale = time.Now().Add(c.duration)
	}
	size := 0
	if c.sizer != nil {
		size = c.sizer(value)
	}
	if c.limit > 0 && len(c.cache) >= c.limit {
		tries := 0
		var now time.Time
		if c.duration != 0 {
			now = time.Now()
		} else {
			tries = 5
		}
		for key, ent := range c.cache {
			if tries < 5 && ent.stale.After(now) {
				tries++
				continue
			}
			c.remove(key, ent)
			break
		}
	}
	if c.sizelimit > 0 {
		if size > c.sizelimit/4 {
			return
		}
		if size+c.size > c.sizelimit {
			for key, ent := range c.cache {
				c.remove(key, ent)
				if size+c.size <= c.sizelimit {
					break
				}
			}
		}
	}
	c.size += size
	c.cache[key] = entry{
		value: value,
		stale: stale,
		size:  size,
	}
}

func (c *Cache) remove(key interface{}, ent entry) {
	c.size -= ent.size
	delete(c.cache, key)
}

// Manually set a cached value.
func (c *Cache) Set(key interface{}, value interface{}) {
	if c.reducer != nil {
		key = c.reducer(key)
	}
	c.lock.Lock()
	c.set(key, value)
	c.lock.Unlock()
}

// Unlock the c, iff lock is held.
func (c *Cache) Unlock() {
	c.lock.Unlock()
}

// Clear one key from the cache
func (c *Cache) Clear(key interface{}) {
	if c.singleton {
		c.Flush()
		return
	}
	if c.reducer != nil {
		key = c.reducer(key)
	}
	c.lock.Lock()
	if ent, ok := c.cache[key]; ok {
		c.serialno++
		c.remove(key, ent)
	}
	c.serializer.Cancel(key)
	c.lock.Unlock()
}

// Flush all values from the cache
func (c *Cache) Flush() {
	c.lock.Lock()
	c.serialno++
	c.cache = make(entrymap)
	c.serializer.CancelAll()
	c.lock.Unlock()
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

type Counter struct {
	cache *Cache
}

func NewCounter(options Options) Counter {
	var c Counter
	c.cache = New(Options{Filler: func(name string) (int64, bool) {
		return 0, true
	}, Duration: options.Duration, Limit: options.Limit})
	return c
}

func (cnt Counter) Get(name string) int64 {
	c := cnt.cache
	var val int64
	c.Get(name, &val)
	return val
}

func (cnt Counter) Inc(name string) int64 {
	c := cnt.cache
	var val int64
	c.GetAndLock(name, &val)
	val++
	c.set(name, val)
	c.Unlock()
	return val
}

func (cnt Counter) Dec(name string) int64 {
	c := cnt.cache
	var val int64
	c.GetAndLock(name, &val)
	val--
	c.set(name, val)
	c.Unlock()
	return val
}
