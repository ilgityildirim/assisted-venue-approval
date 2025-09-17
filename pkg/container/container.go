package container

import (
	"fmt"
	"reflect"
	"sync"
)

// Very small DI container using constructor injection.
// Why: centralize wiring without external deps; keep testable via interfaces.
// It supports:
//  - Provide constructor functions
//  - Singleton scope
//  - Resolve by type and Invoke to call functions with deps
// TODO: add context support if we introduce ctx-bound providers later.

type Container struct {
	mu        sync.RWMutex
	prov      map[reflect.Type]provider
	instances map[reflect.Type]reflect.Value
}

type provider struct {
	fn        reflect.Value
	singleton bool
}

func New() *Container {
	return &Container{prov: make(map[reflect.Type]provider), instances: make(map[reflect.Type]reflect.Value)}
}

// Provide registers a constructor function for a type.
// The constructor may have parameters which will be resolved from the container.
// The function may return either (T) or (T, error).
// If multiple return values are present, the last must be error.
func (c *Container) Provide(constructor interface{}, singleton bool) error {
	v := reflect.ValueOf(constructor)
	if v.Kind() != reflect.Func {
		return fmt.Errorf("container: constructor must be a function")
	}
	// Determine provided type from first return value
	ft := v.Type()
	if ft.NumOut() == 0 || ft.NumOut() > 2 {
		return fmt.Errorf("container: constructor must return (T) or (T, error)")
	}
	outType := ft.Out(0)
	if ft.NumOut() == 2 {
		if ft.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
			return fmt.Errorf("container: second return value must be error")
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.prov[outType]; exists {
		return fmt.Errorf("container: provider already exists for %v", outType)
	}
	c.prov[outType] = provider{fn: v, singleton: singleton}
	return nil
}

// Resolve populates the given pointer with an instance of the requested type.
// Example: var db *DB; c.Resolve(&db)
func (c *Container) Resolve(target interface{}) error {
	ptr := reflect.ValueOf(target)
	if ptr.Kind() != reflect.Ptr || ptr.IsNil() {
		return fmt.Errorf("container: target must be a non-nil pointer")
	}
	want := ptr.Elem().Type()
	val, err := c.get(want, make(map[reflect.Type]bool))
	if err != nil {
		return err
	}
	ptr.Elem().Set(val)
	return nil
}

// Invoke calls a function, resolving its parameters from the container.
func (c *Container) Invoke(fn interface{}) error {
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func {
		return fmt.Errorf("container: Invoke requires a function")
	}
	ft := v.Type()
	args := make([]reflect.Value, ft.NumIn())
	seen := make(map[reflect.Type]bool)
	for i := 0; i < ft.NumIn(); i++ {
		typ := ft.In(i)
		val, err := c.getTyp(typ, seen)
		if err != nil {
			return err
		}
		args[i] = val
	}
	outs := v.Call(args)
	// If last return is error, check it
	if n := len(outs); n > 0 {
		last := outs[n-1]
		if last.IsValid() && last.Type() == reflect.TypeOf((*error)(nil)).Elem() {
			if !last.IsNil() {
				return last.Interface().(error)
			}
		}
	}
	return nil
}

func (c *Container) getTyp(t reflect.Type, seen map[reflect.Type]bool) (reflect.Value, error) {
	// Allow resolving interfaces and concrete types that have providers
	return c.get(t, seen)
}

func (c *Container) get(t reflect.Type, seen map[reflect.Type]bool) (reflect.Value, error) {
	c.mu.RLock()
	// Singleton already built?
	if v, ok := c.instances[t]; ok {
		c.mu.RUnlock()
		return v, nil
	}
	prov, ok := c.prov[t]
	c.mu.RUnlock()
	if !ok {
		// Try to find a provider whose return type implements the requested interface
		c.mu.RLock()
		for pt, p := range c.prov {
			if t.Kind() == reflect.Interface && pt.Implements(t) {
				prov = p
				ok = true
				break
			}
		}
		c.mu.RUnlock()
		if !ok {
			return reflect.Value{}, fmt.Errorf("container: no provider for %v", t)
		}
	}

	// Detect simple cycles
	if seen[t] {
		return reflect.Value{}, fmt.Errorf("container: cyclic dependency for %v", t)
	}
	seen[t] = true

	fn := prov.fn
	ft := fn.Type()
	args := make([]reflect.Value, ft.NumIn())
	for i := 0; i < ft.NumIn(); i++ {
		depType := ft.In(i)
		depVal, err := c.get(depType, seen)
		if err != nil {
			return reflect.Value{}, err
		}
		args[i] = depVal
	}
	outs := fn.Call(args)
	res := outs[0]
	if len(outs) == 2 {
		if err, _ := outs[1].Interface().(error); err != nil {
			return reflect.Value{}, err
		}
	}

	if prov.singleton {
		c.mu.Lock()
		c.instances[t] = res
		c.mu.Unlock()
	}
	return res, nil
}
