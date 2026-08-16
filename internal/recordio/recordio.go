package recordio

import (
	"bytes"
	"fmt"
	"strconv"
)

func Encode(message []byte) []byte {
	header := []byte(strconv.Itoa(len(message)) + "\n")
	return append(header, message...)
}

type Decoder struct {
	buffer []byte
	length int
	failed bool
}

func NewDecoder() *Decoder { return &Decoder{length: -1} }

func (d *Decoder) Decode(data []byte) ([][]byte, error) {
	if d.failed {
		return nil, fmt.Errorf("Decoder is in a FAILED state")
	}
	d.buffer = append(d.buffer, data...)
	var records [][]byte
	for {
		if d.length < 0 {
			newline := bytes.IndexByte(d.buffer, '\n')
			if newline < 0 {
				break
			}
			length, err := strconv.Atoi(string(d.buffer[:newline]))
			if err != nil {
				d.failed = true
				return nil, fmt.Errorf("Failed to decode length'%s': %v", d.buffer[:newline], err)
			}
			d.buffer = d.buffer[newline+1:]
			d.length = length
		}
		if len(d.buffer) < d.length {
			break
		}
		record := append([]byte(nil), d.buffer[:d.length]...)
		records = append(records, record)
		d.buffer = d.buffer[d.length:]
		d.length = -1
	}
	return records, nil
}
