// Package rpc includes constants used by the shiroclient-gateway RPC
// implementation.
// WARNING: this should only be used by the substrate implementation of
// shiroclient, and is subject to breaking changes.
package rpc

const (
	// MethodSeed is used to call the Seed method which re-opens a shiroclient.
	MethodSeed = "Seed"
	// MethodShiroPhylum is used to call the ShiroPhylum method which returns
	// an identifier for the current deployed phylum.
	MethodShiroPhylum = "ShiroPhylum"
	// MethodInit is used to call the Init method which initializes substrate.
	MethodInit = "Init"
	// MethodCall is used to call the Call method which executes a method on
	// the phylum.
	MethodCall = "Call"
	// MethodQueryInfo is used to call the QueryInfo method which returns the
	// blockchain height.
	MethodQueryInfo = "QueryInfo"
	// MethodQueryBlock is used to call the QueryBlock method which returns the
	// block information.
	MethodQueryBlock = "QueryBlock"
)

const (
	// ErrorLevelNoError indicates that no error occurred at any level
	ErrorLevelNoError = iota
	// ErrorLevelShiroClient indicates that an error occurred at the
	// ShiroClient level. That is, the invoked ShiroClient function
	// returned an error.
	ErrorLevelShiroClient
	// ErrorLevelPhylum indicates that the request was passed through to
	// the phylum successfully, but the phylum itself returned an error
	// response.
	ErrorLevelPhylum
)

const (
	// ErrorCodeShiroClientNone indicates no error code.
	ErrorCodeShiroClientNone = iota
	// ErrorCodeShiroClientTimeout indicates the shiro client timed out.
	ErrorCodeShiroClientTimeout
)
