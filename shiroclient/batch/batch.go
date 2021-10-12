package batch

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/sirupsen/logrus"
)

type options struct {
	log       logrus.FieldLogger
	logFields logrus.Fields
}

// Config is a type for a function that can mutate an options object.
type Config func(*options)

// Driver simplifies processing of batch requests originating in
// chaincode accessed through an instance of ShiroClient.
type Driver struct {
	opt    *options
	client shiroclient.ShiroClient
}

// WithLog allows specifying the logger to use.
func WithLog(log logrus.FieldLogger) Config {
	return func(r *options) {
		r.log = log
	}
}

// WithLogField allows specifying a log field to be included.
func WithLogField(key string, value interface{}) Config {
	return func(r *options) {
		r.logFields[key] = value
	}
}

// WithLogrusFields allows specifying multiple log fields to be
// included.
func WithLogrusFields(fields logrus.Fields) Config {
	return func(r *options) {
		for k, v := range fields {
			r.logFields[k] = v
		}
	}
}

const (
	batchGetRequestsMethod     = "batch_get_requests"
	batchProcessResponseMethod = "batch_process_response"
)

func (d *Driver) call(ctx context.Context, method string, params interface{}, batchName string, batchID string, requestID string, clientConfigs ...shiroclient.Config) []byte {
	fields := make(logrus.Fields)
	if batchName != "" {
		fields["batchName"] = batchName
	}
	if batchID != "" {
		fields["batchID"] = batchID
	}
	if requestID != "" {
		fields["requestID"] = requestID
	}
	configs := make([]shiroclient.Config, 0)
	configs = append(configs, shiroclient.WithParams(params), shiroclient.WithLogrusFields(d.opt.logFields), shiroclient.WithLogrusFields(fields), shiroclient.WithParams(params))
	configs = append(configs, clientConfigs...)
	sr, err := d.client.Call(ctx, method, configs...)
	if err != nil {
		d.opt.log.
			WithFields(d.opt.logFields).
			WithFields(fields).
			WithError(err).
			Error("Batch::call: call failed while polling")
		return nil
	}
	if sr.Error() != nil {
		d.opt.log.
			WithFields(d.opt.logFields).
			WithFields(fields).
			WithField("error_code", sr.Error().Code()).
			WithField("error_message", sr.Error().Message()).
			WithField("error_data", string(sr.Error().DataJSON())).
			Error("Batch::call: phylum error while polling")
		return nil
	}
	res := sr.ResultJSON()
	if len(res) == 0 {
		d.opt.log.
			WithFields(d.opt.logFields).
			WithFields(fields).
			Error("Batch::call: empty JSON result while polling")
		return nil
	}
	return res
}

// RequestEnvelope corresponds to the JSON structure used for batch
// requests in the Elps code.
type RequestEnvelope struct {
	BatchID   string          `json:"batch_id"`
	RequestID string          `json:"request_id"`
	Message   json.RawMessage `json:"message"`
}

// ResponseEnvelope corresponds to the JSON structure used for batch
// responses in the Elps code.
type ResponseEnvelope struct {
	BatchID   string          `json:"batch_id"`
	RequestID string          `json:"request_id"`
	IsError   bool            `json:"is_error"`
	Message   json.RawMessage `json:"message"`
}

type callbackFunc func(batchID string, requestID string, message json.RawMessage) (json.RawMessage, error)

// Ticker allows control over batch polling.
type Ticker struct {
	driver        *Driver
	batchName     string
	callback      callbackFunc
	clientConfigs []shiroclient.Config
	ticker        *time.Ticker
	override      chan bool
	// rwMutex guards the enable boolean
	rwMutex *sync.RWMutex
	enable  bool
}

// Tick forces an additional poll right now. This is independent of
// the Pause/Resume mechanism; the poll will happen even if regular
// polling is paused. Additionally, the poll as a whole is synchronous
// - when Tick returns, the last response will have been transacted
// through to the chaincode.
func (t *Ticker) Tick(ctx context.Context) {
	d := t.driver

	res := d.call(ctx, batchGetRequestsMethod, []interface{}{t.batchName}, t.batchName, "", "", t.clientConfigs...)
	if res == nil {
		return
	}

	var envs []RequestEnvelope
	err := json.Unmarshal(res, &envs)
	if err != nil {
		d.opt.log.
			WithFields(d.opt.logFields).
			WithField("batchName", t.batchName).
			WithError(err).
			Error("Batch::Tick: failed to unmarshal while polling")
		return
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	for _, env := range envs {
		env := env
		if env.BatchID == "" || env.RequestID == "" || len(env.Message) == 0 {
			d.opt.log.
				WithFields(d.opt.logFields).
				WithField("batchName", t.batchName).
				Error("Batch::Tick: failed to unmarshal (blank fields) while polling")
			return
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			response, err := t.callback(env.BatchID, env.RequestID, env.Message)
			if err == nil && len(response) == 0 {
				err = errors.New("Batch::Tick: zero-length response")
			}
			if err != nil {
				d.opt.log.
					WithFields(d.opt.logFields).
					WithField("batchName", t.batchName).
					WithField("batchID", env.BatchID).
					WithField("requestID", env.RequestID).
					WithError(err).
					Error("Batch::Tick: callback failed to produce response")
			}

			var isError bool
			var message json.RawMessage

			if err == nil {
				isError = false
				message = response
			} else {
				errError := err.Error()
				isError = true
				message, err = json.Marshal(&errError)
				if err != nil {
					d.opt.log.
						WithFields(d.opt.logFields).
						WithField("batchName", t.batchName).
						WithField("batchID", env.BatchID).
						WithField("requestID", env.RequestID).
						WithError(err).
						Error("Batch::Tick: failed to marshal error response")
					return
				}
			}

			params := []interface{}{
				t.batchName,
				&ResponseEnvelope{
					BatchID:   env.BatchID,
					RequestID: env.RequestID,
					IsError:   isError,
					Message:   message,
				},
			}
			result := d.call(ctx, batchProcessResponseMethod, params, t.batchName, env.BatchID, env.RequestID, t.clientConfigs...)
			if result == nil {
				d.opt.log.
					WithFields(d.opt.logFields).
					WithField("batchName", t.batchName).
					WithField("batchID", env.BatchID).
					WithField("requestID", env.RequestID).
					Error("Batch::Tick: response method failed")
				return
			}

			d.opt.log.WithFields(d.opt.logFields).
				WithField("batchName", t.batchName).
				WithField("batchID", env.BatchID).
				WithField("requestID", env.RequestID).
				Debug("batch processed response")
		}()
	}
}

// TickAsync forces an asynchronous poll. This is independent of the
// Pause/Resume mechanism; the poll will happen even if regular
// polling is paused. It should return (almost) immediately, without
// waiting for the polling and responses to take place.
func (t *Ticker) TickAsync() {
	t.override <- true
}

// Pause pauses regular polling.
func (t *Ticker) Pause() {
	t.rwMutex.Lock()
	defer t.rwMutex.Unlock()

	t.enable = false
}

// Resume resumes regular polling.
func (t *Ticker) Resume() {
	t.rwMutex.Lock()
	defer t.rwMutex.Unlock()

	t.enable = true
}

// Stop permanently stops regular polling.
func (t *Ticker) Stop() {
	t.ticker.Stop()
}

// Register registers a callback for a specific batch name with a
// specific polling interval. Register returns a Ticker that can be
// used to trigger, pause, resume or stop the polling process. The
// callback function can fail to produce a result message, which
// results in a log message. The callback function should take care to
// properly lock any shared state as it will be invoked asynchronously
// w.r.t the "main" thread (or the thread that invoked
// Register). Also, the callback function should return results in a
// reasonable timeframe or return an error, not hang indefinitely.
func (d *Driver) Register(ctx context.Context, batchName string, interval time.Duration, callback func(batchID string, requestID string, message json.RawMessage) (json.RawMessage, error), configs ...shiroclient.Config) *Ticker {
	ticker := &Ticker{
		driver:        d,
		batchName:     batchName,
		callback:      callback,
		clientConfigs: configs,
		ticker:        time.NewTicker(interval),
		override:      make(chan bool),
		rwMutex:       &sync.RWMutex{},
		enable:        true,
	}

	poll := func() {
		for {
			var enable bool

			select {
			case <-ticker.ticker.C:
				ticker.rwMutex.RLock()
				enable = ticker.enable
				ticker.rwMutex.RUnlock()

			case <-ticker.override:
				enable = true
			}

			if !enable {
				continue
			}

			go ticker.Tick(ctx)
		}
	}

	go poll()

	return ticker
}

// NewDriver returns a Driver that will use client as the underlying
// ShiroClient.
func NewDriver(client shiroclient.ShiroClient, configs ...Config) *Driver {
	opt := &options{
		log:       logrus.New(),
		logFields: make(logrus.Fields),
	}

	for _, config := range configs {
		config(opt)
	}

	return &Driver{opt: opt, client: client}
}
