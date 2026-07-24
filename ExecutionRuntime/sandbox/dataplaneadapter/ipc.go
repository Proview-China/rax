package dataplaneadapter

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
)

func readFrame(reader io.Reader, value any) error {
	var length uint32
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
		return err
	}
	if length == 0 || length > maxFrameBytes {
		return errors.New("IPC frame length is outside the closed bounds")
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytesReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("IPC frame contains trailing JSON")
	}
	return nil
}

func writeFrame(writer io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data) > maxFrameBytes {
		return errors.New("IPC frame length is outside the closed bounds")
	}
	if err := binary.Write(writer, binary.BigEndian, uint32(len(data))); err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

func validatePeerUID(connection *net.UnixConn, expected uint32) error {
	raw, err := connection.SyscallConn()
	if err != nil {
		return err
	}
	var credentials *syscall.Ucred
	var socketErr error
	if err := raw.Control(func(fd uintptr) {
		credentials, socketErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return err
	}
	if socketErr != nil {
		return socketErr
	}
	if credentials == nil || credentials.Uid != expected {
		return fmt.Errorf("IPC peer uid is unauthorized")
	}
	return nil
}

type sliceReader struct {
	data   []byte
	offset int
}

func bytesReader(data []byte) *sliceReader { return &sliceReader{data: data} }

func (r *sliceReader) Read(target []byte) (int, error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	count := copy(target, r.data[r.offset:])
	r.offset += count
	return count, nil
}
