package doctor

import (
	"sync"
	"time"
)

type calendar struct {
	// wg waits on HealthChecks complete
	// scheduled executions or the BillOfHealth
	// channel to finish draining
	wg sync.WaitGroup
	c  chan BillOfHealth

	mu    sync.RWMutex
	exams map[string]*appointment
}

func newCalendar() *calendar {
	return &calendar{exams: make(map[string]*appointment)}
}

func (c *calendar) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.exams)
}

func (c *calendar) set(a *appointment) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.exams[a.name] = a
}

func (c *calendar) get(name string) (*appointment, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.exams[name]
	return v, ok
}

func (c *calendar) delete(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	a, ok := c.get(name)
	if ok {
		a.close()
		delete(c.exams, name)
	}
}

func (c *calendar) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, v := range c.exams {
		if v.status.closed {
			continue
		}
		v.close()
	}
}

func (c *calendar) begin() chan BillOfHealth {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, a := range c.exams {
		c.examine(a)
	}
	return c.c
}

func (c *calendar) wait() {
	c.wg.Wait()
}

func (c *calendar) examine(appt *appointment) {

	// set state
	interval := appt.opts.interval
	ttl := appt.opts.ttl

	// add to the WaitGroup before starting the
	// goroutine to avoid wg.Wait() returning
	// before wg.Add(1) can be executed
	c.wg.Add(1)

	go func() {
		if interval < 1 {
			c.run(appt, func() { c.wg.Done() })
			return
		}
		tick := time.NewTicker(interval)
		for {
			select {
			case <-tick.C:
				go c.run(appt)
			case <-appt.status.done:
				tick.Stop()
				c.wg.Done()
				return
			}
		}
	}()

	// if a TTL is set, close the channel at that time
	if ttl > 0 {
		go func() {
			<-time.After(ttl)
			c.delete(appt.name)
		}()
	}
}

// run executes a healthcheck scheduled by an appointment,
// run takes an appointment and an optional callback,
// if you don't need a callback, simply pass a nil value
// as the second parameter
func (c *calendar) run(appt *appointment, callbacks ...func()) {
	c.c <- appt.run() // send the bill of health result
	for _, f := range callbacks {
		f()
	}
}
