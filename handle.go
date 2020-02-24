package lnurl

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"

	"github.com/tidwall/gjson"
)

// HandleLNURL takes a bech32-encoded lnurl and either gets its parameters from the query-
// string or calls the URL to get the parameters.
// Returns a different struct for each of the lnurl subprotocols, the .LNURLKind() method of
// which should be checked next to see how the wallet is going to proceed.
func HandleLNURL(rawlnurl string) (LNURLParams, error) {
	lnurl, ok := FindLNURLInText(rawlnurl)
	if !ok {
		return nil, errors.New("invalid bech32-encoded lnurl: " + rawlnurl)
	}

	rawurl, err := LNURLDecode(lnurl)
	if err != nil {
		return nil, err
	}

	parsed, err := url.Parse(rawurl)
	if err != nil {
		return nil, err
	}

	query := parsed.Query()

	switch query.Get("tag") {
	case "login":
		k1 := query.Get("k1")
		if _, err := hex.DecodeString(k1); err != nil || len(k1) != 64 {
			return nil, errors.New("k1 is not a valid 32-byte hex-encoded string.")
		}

		return LNURLAuthParams{
			Tag:      "login",
			K1:       k1,
			Callback: rawurl,
			Host:     parsed.Host,
		}, nil
	case "withdrawRequest":
		callback := query.Get("callback")
		callbackURL, err := url.Parse(callback)
		if err != nil {
			return nil, fmt.Errorf("'callback' is not a valid URL: %w", err)
		}
		maxWithdrawable, err := strconv.ParseInt(query.Get("maxWithdrawable"), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("'maxWithdrawable' is not a valid integer: %w", err)
		}
		minWithdrawable, err := strconv.ParseInt(query.Get("minWithdrawable"), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("'minWithdrawable' is not a valid integer: %w", err)
		}

		return LNURLWithdrawResponse{
			Tag:                "withdrawRequest",
			K1:                 query.Get("k1"),
			Callback:           callback,
			CallbackURL:        callbackURL,
			MaxWithdrawable:    maxWithdrawable,
			MinWithdrawable:    minWithdrawable,
			DefaultDescription: query.Get("defaultDescription"),
		}, nil
	default:
		resp, err := http.Get(rawurl)
		if err != nil {
			return nil, err
		}

		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		j := gjson.ParseBytes(b)
		if j.Get("status").String() == "ERROR" {
			return nil, errors.New(j.Get("reason").String())
		}

		switch j.Get("tag").String() {
		case "withdrawRequest":
			callback := j.Get("callback").String()
			callbackURL, err := url.Parse(callback)
			if err != nil {
				return nil, errors.New("callback is not a valid URL")
			}

			return LNURLWithdrawResponse{
				Tag:                "withdrawRequest",
				K1:                 j.Get("k1").String(),
				Callback:           callback,
				CallbackURL:        callbackURL,
				MaxWithdrawable:    j.Get("maxWithdrawable").Int(),
				MinWithdrawable:    j.Get("minWithdrawable").Int(),
				DefaultDescription: j.Get("defaultDescription").String(),
			}, nil
		case "payRequest":
			strmetadata := j.Get("metadata").String()
			var metadata [][]string
			err := json.Unmarshal([]byte(strmetadata), &metadata)
			if err != nil {
				return nil, err
			}

			callback := j.Get("callback").String()
			callbackURL, err := url.Parse(callback)
			if err != nil {
				return nil, errors.New("callback is not a valid URL")
			}

			parsedMetadata := make(map[string]string)
			for _, pair := range metadata {
				parsedMetadata[pair[0]] = pair[1]
			}

			return LNURLPayResponse1{
				Tag:             "payRequest",
				Callback:        callback,
				CallbackURL:     callbackURL,
				EncodedMetadata: strmetadata,
				Metadata:        metadata,
				ParsedMetadata:  parsedMetadata,
				MaxSendable:     j.Get("maxSendable").Int(),
				MinSendable:     j.Get("minSendable").Int(),
			}, nil
		case "channelRequest":
			k1 := j.Get("k1").String()
			if k1 == "" {
				return nil, errors.New("k1 is blank")
			}
			callback := j.Get("callback").String()
			callbackURL, err := url.Parse(callback)
			if err != nil {
				return nil, errors.New("callback is not a valid URL")
			}

			return LNURLChannelResponse{
				Tag:         "channelRequest",
				K1:          k1,
				Callback:    callback,
				CallbackURL: callbackURL,
				URI:         j.Get("uri").String(),
			}, nil
		default:
			return nil, errors.New("unknown response tag " + j.String())
		}
	}
}
