//
// Copyright (c) SAS Institute Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package pkcs

// Verify PKCS#7 SignedData structures. Also includes shared code for
// serializing other signature types that use PKCS#7.

import (
	"crypto"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/sassoftware/relic/config"
	"github.com/sassoftware/relic/lib/magic"
	"github.com/sassoftware/relic/lib/pkcs7"
	"github.com/sassoftware/relic/lib/pkcs9"
	"github.com/sassoftware/relic/lib/x509tools"
	"github.com/sassoftware/relic/signers"
)

var PkcsSigner = &signers.Signer{
	Name:      "pkcs7",
	Magic:     magic.FileTypePKCS7,
	CertTypes: signers.CertTypeX509,
	Sign:      nil,
	Verify:    Verify,
}

func init() {
	PkcsSigner.Flags().String("content", "", "Specify file containing contents for detached signatures")
	signers.Register(PkcsSigner)
}

type Timestamper struct {
	Config *config.TimestampConfig
}

func (t Timestamper) Timestamp(data []byte, hash crypto.Hash) (*pkcs7.ContentInfoSignedData, error) {
	if t.Config == nil {
		return nil, nil
	}
	d := hash.New()
	d.Write(data)
	imprint := d.Sum(nil)

	cl := pkcs9.TimestampClient{
		UserAgent: config.UserAgent,
		CaFile:    t.Config.CaCert,
		Timeout:   time.Second * time.Duration(t.Config.Timeout),
	}
	var token *pkcs7.ContentInfoSignedData
	var err error
	for _, url := range t.Config.URLs {
		cl.URL = url
		if err != nil {
			fmt.Fprintf(os.Stderr, "Timestamping failed: %s\nTrying next server %s...\n", err, cl.URL)
		}
		token, err = cl.Request(hash, imprint)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("Timestamping failed: %s", err)
	}
	return token, nil
}

func Verify(f *os.File, opts signers.VerifyOpts) ([]*signers.Signature, error) {
	blob, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	psd, err := pkcs7.Unmarshal(blob)
	if err != nil {
		return nil, err
	}
	var cblob []byte
	if !opts.NoDigests && opts.Content != "" {
		cblob, err = ioutil.ReadFile(opts.Content)
		if err != nil {
			return nil, err
		}
	}
	sig, err := psd.Content.Verify(cblob, opts.NoDigests)
	if err != nil {
		return nil, err
	}
	ts, err := pkcs9.VerifyOptionalTimestamp(sig)
	if err != nil {
		return nil, err
	}
	hash, _ := x509tools.PkixDigestToHash(ts.SignerInfo.DigestAlgorithm)
	return []*signers.Signature{&signers.Signature{
		Hash:          hash,
		X509Signature: &ts,
	}}, nil
}
