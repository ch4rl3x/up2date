package mqtt

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"
)

type Client struct {
	conn net.Conn
}

func Dial(
	ctx context.Context,
	host string,
	port int,
	clientID string,
	username string,
	password string,
	timeout time.Duration,
) (*Client, error) {
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, fmt.Errorf("connect to mqtt broker: %w", err)
	}

	client := &Client{conn: conn}
	if err := client.connect(clientID, username, password, timeout); err != nil {
		conn.Close()
		return nil, err
	}
	return client, nil
}

func (c *Client) Publish(ctx context.Context, topic string, payload []byte, retain bool) error {
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.conn.SetWriteDeadline(deadline)
	} else {
		_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	}

	header := byte(0x30)
	if retain {
		header |= 0x01
	}

	variableHeader := encodeString(topic)
	packetBody := append(variableHeader, payload...)
	packet := append([]byte{header}, encodeRemainingLength(len(packetBody))...)
	packet = append(packet, packetBody...)

	if _, err := c.conn.Write(packet); err != nil {
		return fmt.Errorf("publish mqtt packet: %w", err)
	}
	return nil
}

func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	_, _ = c.conn.Write([]byte{0xE0, 0x00})
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *Client) connect(clientID, username, password string, timeout time.Duration) error {
	_ = c.conn.SetDeadline(time.Now().Add(timeout))
	defer c.conn.SetDeadline(time.Time{})

	flags := byte(0x02)
	payload := make([]byte, 0, 128)
	payload = append(payload, encodeString(clientID)...)
	if username != "" {
		flags |= 0x80
		payload = append(payload, encodeString(username)...)
	}
	if password != "" {
		flags |= 0x40
		payload = append(payload, encodeString(password)...)
	}

	variableHeader := make([]byte, 0, 16)
	variableHeader = append(variableHeader, encodeString("MQTT")...)
	variableHeader = append(variableHeader, 0x04)
	variableHeader = append(variableHeader, flags)
	variableHeader = append(variableHeader, 0x00, 0x1E)

	packetBody := append(variableHeader, payload...)
	packet := append([]byte{0x10}, encodeRemainingLength(len(packetBody))...)
	packet = append(packet, packetBody...)

	if _, err := c.conn.Write(packet); err != nil {
		return fmt.Errorf("send mqtt connect packet: %w", err)
	}

	packetType, body, err := readPacket(c.conn)
	if err != nil {
		return fmt.Errorf("read mqtt connack: %w", err)
	}
	if packetType != 0x20 || len(body) != 2 {
		return fmt.Errorf("broker returned an unexpected connack packet")
	}
	if body[1] != 0x00 {
		return fmt.Errorf("broker rejected connection with code %d", body[1])
	}
	return nil
}

func readPacket(reader io.Reader) (byte, []byte, error) {
	var fixedHeader [1]byte
	if _, err := io.ReadFull(reader, fixedHeader[:]); err != nil {
		return 0, nil, err
	}

	remainingLength := 0
	multiplier := 1
	for {
		var encoded [1]byte
		if _, err := io.ReadFull(reader, encoded[:]); err != nil {
			return 0, nil, err
		}
		remainingLength += int(encoded[0]&0x7F) * multiplier
		if encoded[0]&0x80 == 0 {
			break
		}
		multiplier *= 128
		if multiplier > 128*128*128 {
			return 0, nil, fmt.Errorf("malformed mqtt remaining length")
		}
	}

	body := make([]byte, remainingLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		return 0, nil, err
	}
	return fixedHeader[0] & 0xF0, body, nil
}

func encodeString(value string) []byte {
	raw := []byte(value)
	packet := make([]byte, 0, len(raw)+2)
	packet = append(packet, byte(len(raw)>>8), byte(len(raw)))
	packet = append(packet, raw...)
	return packet
}

func encodeRemainingLength(value int) []byte {
	encoded := make([]byte, 0, 4)
	for {
		digit := value % 128
		value /= 128
		if value > 0 {
			digit |= 0x80
		}
		encoded = append(encoded, byte(digit))
		if value == 0 {
			break
		}
	}
	return encoded
}
