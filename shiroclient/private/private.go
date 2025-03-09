// Package private enables the secure processing of PII data within substrate.
// It provides helpers to encrypt and decrypt data that is sent to substrate
// for subsequent processing, as well as purging data belonging to an
// individual.
package private

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
)

const (
	// ShiroEndpointDecode is used to decode private data.
	ShiroEndpointDecode = "private_decode"
	// ShiroEndpointEncode is used to encode private data.
	ShiroEndpointEncode = "private_encode"
	// ShiroEndpointPurge is used to purge private data from the blockchain for
	// a data subject.
	ShiroEndpointPurge = "private_purge"
	// ShiroEndpointExport is used to export a data subject's private data.
	ShiroEndpointExport = "private_export"
	// ShiroEndpointProfileToDSID is used to get a DSID given a profile.
	ShiroEndpointProfileToDSID = "private_get_dsid"
)

const (
	hkdfSeedSize = 32
)

// SeedGen generates random secret keys. This is a hook that can be overridden
// at run time.
var SeedGen = func() ([]byte, error) {
	key := make([]byte, hkdfSeedSize)
	_, err := rand.Read(key)
	if err != nil {
		return nil, err
	}
	return key, nil
}

// Encryptor selects message transform encryption algorithms.
type Encryptor string

// EncryptorNone indicates that no encryption should be applied.
const EncryptorNone Encryptor = "none"

// EncryptorAES256 indicates that AES-256 encryption should be applied.
const EncryptorAES256 Encryptor = "AES-256"

// Compressor selects message transform compression algortihms.
type Compressor string

// CompressorNone indicates that no compression should be applied.
const CompressorNone Compressor = "none"

// CompressorZlib indicates that zlib compression should be applied.
const CompressorZlib Compressor = "zlib"

// DSID is an identifier that represents a Data Subject.
type DSID string

// TransformHeader is a header for a message transformation.
// This is exported for json serialization.
type TransformHeader struct {
	// ProfilePaths are elpspaths that compose a data subject profile.
	ProfilePaths []string `json:"profile_paths"`
	// PrivatePaths are elpspaths that select private data.
	PrivatePaths []string `json:"private_paths"`
	// Encryptor selects the encryption algorithm.
	Encryptor Encryptor `json:"encryptor"`
	// Compressor selects the compression algorithm.
	Compressor Compressor `json:"compressor"`
}

// TransformBody is the body portion of a transformation. This is populated
// on encoded messages.
// This is exported for json serialization.
type TransformBody struct {
	// DSID is the data subject ID for the encoded transformation.
	DSID DSID `json:"dsid"`
	// EncryptedBase64 is the encrypted bytes belonging to the data subject.
	EncryptedBase64 string `json:"encrypted_base64"`
}

// Transform is a message transformation. It encapsulates both transformed
// messages (body), as well as settings to perform a transformation (header).
type Transform struct {
	// ContextPath represents an elpspath within the message where the
	// transformation will be applied. All transformation paths are relative
	// to this context.
	ContextPath string `json:"context_path"`
	// Header represents a transformation header. It is a description of
	// the transformation used for encoding and decoding.
	Header *TransformHeader `json:"header"`
	// Body includes an encoded message, where the encoding used the settings
	// defined in the Header.
	Body *TransformBody `json:"body"`
}

// EncodedMessage is a message that has undergone encoding.
// This is exported for json serialization.
type EncodedMessage struct {
	// MXF is a sentinel to indicate the message was encoded using libmxf.
	MXF string `json:"mxf"`
	// Message is the plaintext part of an encoded message.
	Message interface{} `json:"message"`
	// Transforms are the applied transforms.
	Transforms []*Transform `json:"transforms"`
}

// EncodeRequest is a request to encode a message.
// This is exported for json serialization.
type EncodeRequest struct {
	// Message is the message to be encoded.
	Message interface{} `json:"message"`
	// Transforms are the transformations to apply.
	Transforms []*Transform `json:"transforms"`
}

// EncodedResponse is a result of encoding a message, and can subsequently
// be decoded.
type EncodedResponse struct {
	// EncodedMessage is only set to the encoded message if encode did perform
	// encoding.
	encodedMessage *EncodedMessage
	// RawMessage is only set to the raw response if encode did not actually
	// perform any encoding.
	rawMessage *json.RawMessage
}

// MarshalJSON implements json.Marshaler.
func (r *EncodedResponse) MarshalJSON() ([]byte, error) {
	if r.encodedMessage == nil && r.rawMessage == nil {
		return nil, errors.New("empty response")
	}
	if r.encodedMessage == nil {
		return json.Marshal(r.rawMessage)
	}
	return json.Marshal(r.encodedMessage)
}

// UnmarshalJSON implements json.Unmarshaler.
func (r *EncodedResponse) UnmarshalJSON(b []byte) error {
	encMsg := &EncodedMessage{}
	err := json.Unmarshal(b, encMsg)
	if err != nil || encMsg.MXF == "" {
		raw := &json.RawMessage{}
		err = json.Unmarshal(b, raw)
		if err != nil {
			return err
		}
		r.rawMessage = raw
	} else {
		r.encodedMessage = encMsg
	}
	return nil
}

var skipEncodeConfig = shiroclient.WithSingleton()

// WithSkipEncodeTx skips the encode transaction and instead encodes the
// private data in the same transaction as the wrapped Call transaction. This
// is an optimization to reduce the number of transactions.
func WithSkipEncodeTx() shiroclient.Config {
	return skipEncodeConfig
}

func doSkipEncodeTx(configs []shiroclient.Config) bool {
	for _, config := range configs {
		if config == skipEncodeConfig {
			return true
		}
	}
	return false
}

// withParam returns a shiroclient config that passes a single parameter
// as an argument to an endpoint.
func withParam(arg interface{}) shiroclient.Config {
	return shiroclient.WithParams([]interface{}{arg})
}

// WithSeed returns a shiroclient config that includes a CSPRNG seed.
func WithSeed() (shiroclient.Config, error) {
	seed, err := SeedGen()
	if err != nil {
		return nil, err
	}
	return shiroclient.WithTransientData("csprng_seed_private", seed), nil
}

// WithTransientMXF adds transient data used by MXF to encode and encrypt data.
// This config is not compatible with `WithTransientIVs`.
func WithTransientMXF(req *EncodeRequest) ([]shiroclient.Config, error) {
	if req == nil {
		req = &EncodeRequest{}
	}
	var configs []shiroclient.Config
	seedConfig, err := WithSeed()
	if err != nil {
		return nil, err
	}
	configs = append(configs, seedConfig)
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	configs = append(configs, shiroclient.WithTransientData("mxf", reqBytes))
	return configs, nil
}

func encodeHelper(ctx context.Context, client shiroclient.ShiroClient, message interface{}, transforms []*Transform, configs ...shiroclient.Config) (*EncodedResponse, []shiroclient.Config, error) {
	if message == nil {
		return nil, nil, nil
	}
	var newConfigs []shiroclient.Config
	if len(transforms) == 0 {
		// fast path, nothing to do.
		rawBytes, err := json.Marshal(message)
		if err != nil {
			return nil, nil, err
		}
		encResp := &EncodedResponse{}
		err = json.Unmarshal(rawBytes, encResp)
		if err != nil {
			return nil, nil, err
		}

		newConfigs = append(newConfigs, withParam(encResp))
		return encResp, newConfigs, nil
	}

	transientConfigs, err := WithTransientMXF(&EncodeRequest{
		Message:    message,
		Transforms: transforms,
	})
	if err != nil {
		return nil, nil, err
	}

	enc := &EncodedResponse{}
	if doSkipEncodeTx(configs) {
		newConfigs = append(newConfigs, transientConfigs...)
		// for this optimization, pass a hard coded "magic" request that tells
		// `substrate` to look for the to-be encoded message in transient data.
		newConfigs = append(newConfigs, withParam(skipEncodeRequest))
	} else {

		configs = append(configs, transientConfigs...)

		resp, err := client.Call(ctx, ShiroEndpointEncode, configs...)
		if err != nil {
			return nil, nil, err
		}

		if resp.Error() != nil {
			return nil, nil, errors.New(resp.Error().Message())
		}
		err = resp.UnmarshalTo(enc)
		if err != nil {
			return nil, nil, err
		}

		newConfigs = append(newConfigs, shiroclient.WithDependentTxID(resp.TransactionID()))
		newConfigs = append(newConfigs, withParam(enc))
	}

	return enc, newConfigs, nil
}

// Encode encodes a sensitive "message" using "transforms".
// If there no transforms, then encode simply returns a thin wrapper
// over the encoded message bytes.
func Encode(ctx context.Context, client shiroclient.ShiroClient, message interface{}, transforms []*Transform, configs ...shiroclient.Config) (*EncodedResponse, error) {
	enc, _, err := encodeHelper(ctx, client, message, transforms, configs...)
	if err != nil {
		return nil, err
	}
	return enc, nil
}

// Decode decodes a message that was encoded with transforms. If there are
// no transforms, then decode unmarshals the raw message bytes into "decoded".
func Decode(ctx context.Context, client shiroclient.ShiroClient, encoded *EncodedResponse, decoded interface{}, configs ...shiroclient.Config) error {
	if encoded == nil {
		return errors.New("nil encoded message")
	}
	if encoded.encodedMessage == nil {
		// fast path, nothing to do.
		if encoded.rawMessage == nil {
			return errors.New("missing raw message")
		}
		rawBytes, err := json.Marshal(encoded.rawMessage)
		if err != nil {
			return err
		}
		return shiroclient.UnmarshalProto(rawBytes, decoded)
	}
	configs = append(configs, withParam(encoded.encodedMessage))
	resp, err := client.Call(ctx, ShiroEndpointDecode, configs...)
	if err != nil {
		return err
	}
	if resp.Error() != nil {
		return errors.New(resp.Error().Message())
	}
	err = resp.UnmarshalTo(decoded)
	if err != nil {
		return err
	}
	return nil
}

// Export exports all sensitive data on the blockchain pertaining to a data
// subject with data subject ID "dsid".
func Export(ctx context.Context, client shiroclient.ShiroClient, dsid DSID, configs ...shiroclient.Config) (map[string]interface{}, error) {
	if dsid == "" {
		return nil, errors.New("invalid empty DSID")
	}
	configs = append(configs, withParam(dsid))
	resp, err := client.Call(ctx, ShiroEndpointExport, configs...)
	if err != nil {
		return nil, err
	}
	if resp.Error() != nil {
		return nil, errors.New(resp.Error().Message())
	}
	var exported map[string]interface{}
	err = resp.UnmarshalTo(&exported)
	if err != nil {
		return nil, err
	}
	return exported, nil
}

// Purge removes all sensitive data on the blockchain pertaining to a data
// subject with data subject ID "dsid".
func Purge(ctx context.Context, client shiroclient.ShiroClient, dsid DSID, configs ...shiroclient.Config) error {
	if dsid == "" {
		return errors.New("invalid empty DSID")
	}
	configs = append(configs, withParam(dsid))
	seedConfig, err := WithSeed()
	if err != nil {
		return err
	}
	configs = append(configs, seedConfig)
	resp, err := client.Call(ctx, ShiroEndpointPurge, configs...)
	if err != nil {
		return err
	}
	if resp.Error() != nil {
		return errors.New(resp.Error().Message())
	}
	var gotDSID DSID
	err = resp.UnmarshalTo(&gotDSID)
	if err != nil {
		return err
	}
	if gotDSID != dsid {
		return fmt.Errorf("unexpected response from purge: got %s != expected %s", gotDSID, dsid)
	}
	return nil
}

// ProfileToDSID returns a DSID for a data subject profile.
func ProfileToDSID(ctx context.Context, client shiroclient.ShiroClient, profile interface{}, configs ...shiroclient.Config) (DSID, error) {
	configs = append(configs, withParam(profile))
	resp, err := client.Call(ctx, ShiroEndpointProfileToDSID, configs...)
	if err != nil {
		return "", err
	}
	if resp.Error() != nil {
		return "", errors.New(resp.Error().Message())
	}
	var gotDSID DSID
	err = resp.UnmarshalTo(&gotDSID)
	if err != nil {
		return "", err
	}
	return gotDSID, nil
}

// CallResult is returned from wrapped calls and contains additional data
// relating to the response.
type CallResult struct {
	TransactionID  string
	maxSimBlockNum uint64
	commitBlockNum uint64
}

// MaxSimBlockNum returns the maximum block number used to simulate the tx
// for the wrapped Call function.
func (s *CallResult) MaxSimBlockNum() uint64 {
	if s == nil {
		return 0
	}
	return s.maxSimBlockNum
}

// CommitBlockNum returns the block number used to commit the tx, or
// empty string if not available.
func (s *CallResult) CommitBlockNum() uint64 {
	if s == nil {
		return 0
	}
	return s.commitBlockNum
}

// CallFunc is the function signature returned for wrapped calls
type CallFunc func(
	ctx context.Context,
	message interface{},
	output interface{},
	configs ...shiroclient.Config) (*CallResult, error)

var skipEncodeRequest = &EncodedResponse{
	encodedMessage: &EncodedMessage{
		// mxf version "transient" is a special constant used to indicate that
		// the encoded message can be found instead in the transient data
		// `mxf` field.
		MXF: "transient",
	},
}

// WrapCall wraps a shiro call. If the transaction logic encrypts new data
// then IVs must be specified, via the `WithTransientIVs` function.
// The configs passed to this are passed to the wrapped call, and not the
// encode and decode operations. This is to prevent the caller from accidently
// overwriting the transient data fields.
// If the caller passes "WithParam" explicitly then this will be ignored in
// favor of the `message`.
// IMPORTANT: The wrapper assumes the wrapped endpoint only takes a single
// argument!
func WrapCall(client shiroclient.ShiroClient, method string, encTransforms ...*Transform) CallFunc {
	return func(ctx context.Context, message interface{}, output interface{}, configs ...shiroclient.Config) (*CallResult, error) {
		_, newConfigs, err := encodeHelper(ctx, client, message, encTransforms, configs...)
		if err != nil {
			return nil, fmt.Errorf("wrap encode error: %w", err)
		}
		callConfigs := append(configs, newConfigs...)
		resp, err := client.Call(ctx, method, callConfigs...)
		if err != nil {
			return nil, fmt.Errorf("wrap call error: %w", err)
		}
		if resp.Error() != nil {
			return nil, fmt.Errorf("wrap call response error: %s", resp.Error().Message())
		}
		encResp := &EncodedResponse{}
		err = resp.UnmarshalTo(encResp)
		if err != nil {
			return nil, err
		}
		if resp.TransactionID() != "" {
			configs = append(configs, shiroclient.WithDependentTxID(resp.TransactionID()))
		}
		err = Decode(ctx, client, encResp, output, configs...)
		if err != nil {
			return nil, fmt.Errorf("wrap decode error: %w", err)
		}
		return &CallResult{
			TransactionID:  resp.TransactionID(),
			maxSimBlockNum: resp.MaxSimBlockNum(),
			commitBlockNum: resp.CommitBlockNum(),
		}, nil
	}
}
