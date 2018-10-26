package signalfx

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"

	"github.com/gogo/protobuf/types"
	policy "istio.io/api/policy/v1beta1"
)

// Ripped from https://sosedoff.com/2014/12/15/generate-random-hex-string-in-go.html
func randomHex(n int) (string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// Ripped from https://github.com/vmware/wavefront-adapter-for-istio/blob/master/wavefront/wavefront.go
func decodeValue(in *policy.Value) interface{} {
	if in == nil {
		return nil
	}

	switch t := in.Value.(type) {
	case *policy.Value_StringValue:
		return t.StringValue
	case *policy.Value_Int64Value:
		return t.Int64Value
	case *policy.Value_DoubleValue:
		return t.DoubleValue
	case *policy.Value_BoolValue:
		return t.BoolValue
	case *policy.Value_IpAddressValue:
		return (net.IP)(t.IpAddressValue.Value)
	case *policy.Value_TimestampValue:
		return t.TimestampValue
	case *policy.Value_DurationValue:
		v, err := types.DurationFromProto(t.DurationValue.Value)
		if err != nil {
			return 0
		}
		return v
	case *policy.Value_EmailAddressValue:
		return t.EmailAddressValue
	case *policy.Value_DnsNameValue:
		return t.DnsNameValue
	case *policy.Value_UriValue:
		return t.UriValue
	default:
		return fmt.Sprintf("%v", in)
	}
}
