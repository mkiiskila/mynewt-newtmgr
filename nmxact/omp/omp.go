/**
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package omp

import (
	"encoding/hex"
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/fatih/structs"
	"github.com/runtimeco/go-coap"
	"github.com/ugorji/go/codec"

	"mynewt.apache.org/newtmgr/nmxact/nmp"
)

// OIC wrapping adds this many bytes to an NMP message.  Calculated by
// comparing an OMP message with its NMP equivalent.
const OMP_MSG_OVERHEAD = 13

type OicMsg struct {
	Hdr []byte `codec:"_h"`
}

/*
 * Not able to install custom decoder for indefite length objects with the
 * codec.  So we need to decode the whole response, and then re-encode the
 * newtmgr response part.
 */
func DecodeOmp(m coap.Message) (nmp.NmpRsp, error) {
	// Ignore non-responses.
	if m.Code() == coap.GET || m.Code() == coap.PUT {
		return nil, nil
	}

	if m.Code() != coap.Created && m.Code() != coap.Deleted &&
		m.Code() != coap.Valid && m.Code() != coap.Changed &&
		m.Code() != coap.Content {
		return nil, fmt.Errorf(
			"OMP response specifies unexpected code: %d (%s)", int(m.Code()),
			m.Code().String())
	}

	var om OicMsg
	err := codec.NewDecoderBytes(m.Payload(), new(codec.CborHandle)).Decode(&om)
	if err != nil {
		return nil, fmt.Errorf("Invalid incoming cbor: %s", err.Error())
	}
	if om.Hdr == nil {
		return nil, fmt.Errorf("Invalid incoming OMP response; NMP header" +
			"missing")
	}

	hdr, err := nmp.DecodeNmpHdr(om.Hdr)
	if err != nil {
		return nil, err
	}

	rsp, err := nmp.DecodeRspBody(hdr, m.Payload())
	if err != nil {
		return nil, fmt.Errorf("Error decoding OMP response: %s", err.Error())
	}
	if rsp == nil {
		return nil, nil
	}

	return rsp, nil
}

type encodeRecord struct {
	tm       *coap.TcpMessage
	hdrBytes []byte
	fieldMap map[string]interface{}
}

func encodeOmpBase(nmr *nmp.NmpMsg) (encodeRecord, error) {
	er := encodeRecord{
		tm: coap.NewTcpMessage(coap.MessageParams{
			Type: coap.Confirmable,
			Code: coap.PUT,
		}),
	}
	er.tm.SetPathString("/omgr")

	payload := []byte{}
	enc := codec.NewEncoderBytes(&payload, new(codec.CborHandle))

	fields := structs.Fields(nmr.Body)
	er.fieldMap = make(map[string]interface{}, len(fields))
	for _, f := range fields {
		if cname := f.Tag("codec"); cname != "" {
			er.fieldMap[cname] = f.Value()
		}
	}

	// Add the NMP heder to the OMP response map.
	er.hdrBytes = nmr.Hdr.Bytes()
	er.fieldMap["_h"] = er.hdrBytes

	if err := enc.Encode(er.fieldMap); err != nil {
		return er, err
	}
	er.tm.SetPayload(payload)

	return er, nil
}

func EncodeOmpTcp(nmr *nmp.NmpMsg) ([]byte, error) {
	er, err := encodeOmpBase(nmr)
	if err != nil {
		return nil, err
	}

	data, err := er.tm.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("Failed to encode: %s\n", err.Error())
	}

	log.Debugf("Serialized OMP TCP request:\n"+
		"Hdr %+v:\n%s\nPayload:%s\nData:\n%s",
		nmr.Hdr, hex.Dump(er.hdrBytes), hex.Dump(er.tm.Payload()),
		hex.Dump(data))

	return data, nil
}

func EncodeOmpDgram(nmr *nmp.NmpMsg) ([]byte, error) {
	er, err := encodeOmpBase(nmr)
	if err != nil {
		return nil, err
	}

	data, err := er.tm.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("Failed to encode: %s\n", err.Error())
	}

	log.Debugf("Serialized OMP datagram request:\n"+
		"Hdr %+v:\n%s\nPayload:%s\nData:\n%s",
		nmr.Hdr, hex.Dump(er.hdrBytes), hex.Dump(er.tm.Payload()),
		hex.Dump(data))

	return data, nil
}
