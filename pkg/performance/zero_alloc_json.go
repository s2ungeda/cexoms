package performance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"unsafe"
)

// JSONWriter provides zero-allocation JSON writing
type JSONWriter struct {
	buf        *Buffer
	bufferPool *BufferPool
	scratch    [64]byte // Scratch space for number formatting
}

// NewJSONWriter creates a new JSON writer
func NewJSONWriter(bufferPool *BufferPool) *JSONWriter {
	return &JSONWriter{
		buf:        bufferPool.Get(1024),
		bufferPool: bufferPool,
	}
}

// Reset resets the writer for reuse
func (w *JSONWriter) Reset() {
	w.buf.Reset()
}

// Release returns the buffer to the pool
func (w *JSONWriter) Release() {
	if w.buf != nil && w.bufferPool != nil {
		w.bufferPool.Put(w.buf)
		w.buf = nil
	}
}

// Bytes returns the written bytes
func (w *JSONWriter) Bytes() []byte {
	return w.buf.Bytes()
}

// String returns the written data as a string without allocation
func (w *JSONWriter) String() string {
	return bytesToString(w.buf.Bytes())
}

// WriteObjectStart writes '{'
func (w *JSONWriter) WriteObjectStart() {
	w.buf.B = append(w.buf.B, '{')
}

// WriteObjectEnd writes '}'
func (w *JSONWriter) WriteObjectEnd() {
	w.buf.B = append(w.buf.B, '}')
}

// WriteArrayStart writes '['
func (w *JSONWriter) WriteArrayStart() {
	w.buf.B = append(w.buf.B, '[')
}

// WriteArrayEnd writes ']'
func (w *JSONWriter) WriteArrayEnd() {
	w.buf.B = append(w.buf.B, ']')
}

// WriteComma writes ','
func (w *JSONWriter) WriteComma() {
	w.buf.B = append(w.buf.B, ',')
}

// WriteColon writes ':'
func (w *JSONWriter) WriteColon() {
	w.buf.B = append(w.buf.B, ':')
}

// WriteString writes a JSON string with proper escaping
func (w *JSONWriter) WriteString(s string) {
	w.buf.B = append(w.buf.B, '"')
	w.buf.B = appendEscapedString(w.buf.B, s)
	w.buf.B = append(w.buf.B, '"')
}

// WriteStringBytes writes a JSON string from bytes
func (w *JSONWriter) WriteStringBytes(b []byte) {
	w.buf.B = append(w.buf.B, '"')
	w.buf.B = appendEscapedBytes(w.buf.B, b)
	w.buf.B = append(w.buf.B, '"')
}

// WriteInt writes an integer
func (w *JSONWriter) WriteInt(i int64) {
	w.buf.B = strconv.AppendInt(w.buf.B, i, 10)
}

// WriteUint writes an unsigned integer
func (w *JSONWriter) WriteUint(u uint64) {
	w.buf.B = strconv.AppendUint(w.buf.B, u, 10)
}

// WriteFloat writes a float
func (w *JSONWriter) WriteFloat(f float64) {
	w.buf.B = strconv.AppendFloat(w.buf.B, f, 'f', -1, 64)
}

// WriteBool writes a boolean
func (w *JSONWriter) WriteBool(b bool) {
	if b {
		w.buf.B = append(w.buf.B, "true"...)
	} else {
		w.buf.B = append(w.buf.B, "false"...)
	}
}

// WriteNull writes null
func (w *JSONWriter) WriteNull() {
	w.buf.B = append(w.buf.B, "null"...)
}

// WriteKey writes a JSON object key
func (w *JSONWriter) WriteKey(key string) {
	w.WriteString(key)
	w.WriteColon()
}

// WriteRaw writes raw bytes without escaping
func (w *JSONWriter) WriteRaw(b []byte) {
	w.buf.B = append(w.buf.B, b...)
}

// appendEscapedString appends an escaped string without allocation
func appendEscapedString(dst []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"', '\\':
			dst = append(dst, '\\', c)
		case '\b':
			dst = append(dst, '\\', 'b')
		case '\f':
			dst = append(dst, '\\', 'f')
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		default:
			if c < 0x20 {
				dst = append(dst, '\\', 'u', '0', '0')
				dst = append(dst, hexDigits[c>>4], hexDigits[c&0xf])
			} else {
				dst = append(dst, c)
			}
		}
	}
	return dst
}

// appendEscapedBytes appends escaped bytes without allocation
func appendEscapedBytes(dst []byte, b []byte) []byte {
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch c {
		case '"', '\\':
			dst = append(dst, '\\', c)
		case '\b':
			dst = append(dst, '\\', 'b')
		case '\f':
			dst = append(dst, '\\', 'f')
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		default:
			if c < 0x20 {
				dst = append(dst, '\\', 'u', '0', '0')
				dst = append(dst, hexDigits[c>>4], hexDigits[c&0xf])
			} else {
				dst = append(dst, c)
			}
		}
	}
	return dst
}

var hexDigits = []byte("0123456789abcdef")

// bytesToString converts bytes to string without allocation
func bytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// stringToBytes converts string to bytes without allocation
func stringToBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// JSONReader provides zero-allocation JSON reading
type JSONReader struct {
	data   []byte
	offset int
}

// NewJSONReader creates a new JSON reader
func NewJSONReader(data []byte) *JSONReader {
	return &JSONReader{
		data: data,
	}
}

// Reset resets the reader with new data
func (r *JSONReader) Reset(data []byte) {
	r.data = data
	r.offset = 0
}

// skipWhitespace skips whitespace characters
func (r *JSONReader) skipWhitespace() {
	for r.offset < len(r.data) {
		c := r.data[r.offset]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		r.offset++
	}
}

// ReadString reads a JSON string without allocation
func (r *JSONReader) ReadString() (string, error) {
	r.skipWhitespace()
	
	if r.offset >= len(r.data) || r.data[r.offset] != '"' {
		return "", fmt.Errorf("expected string")
	}
	
	r.offset++ // Skip opening quote
	start := r.offset
	
	for r.offset < len(r.data) {
		if r.data[r.offset] == '"' && (r.offset == start || r.data[r.offset-1] != '\\') {
			result := bytesToString(r.data[start:r.offset])
			r.offset++ // Skip closing quote
			return result, nil
		}
		r.offset++
	}
	
	return "", fmt.Errorf("unterminated string")
}

// ReadNumber reads a JSON number
func (r *JSONReader) ReadNumber() (float64, error) {
	r.skipWhitespace()
	
	start := r.offset
	
	// Find end of number
	for r.offset < len(r.data) {
		c := r.data[r.offset]
		if (c >= '0' && c <= '9') || c == '-' || c == '+' || c == '.' || c == 'e' || c == 'E' {
			r.offset++
		} else {
			break
		}
	}
	
	if start == r.offset {
		return 0, fmt.Errorf("expected number")
	}
	
	return strconv.ParseFloat(bytesToString(r.data[start:r.offset]), 64)
}

// ReadBool reads a JSON boolean
func (r *JSONReader) ReadBool() (bool, error) {
	r.skipWhitespace()
	
	if r.offset+4 <= len(r.data) && bytesToString(r.data[r.offset:r.offset+4]) == "true" {
		r.offset += 4
		return true, nil
	}
	
	if r.offset+5 <= len(r.data) && bytesToString(r.data[r.offset:r.offset+5]) == "false" {
		r.offset += 5
		return false, nil
	}
	
	return false, fmt.Errorf("expected boolean")
}

// FastJSONEncoder provides fast JSON encoding for common types
type FastJSONEncoder struct {
	pool *BufferPool
}

// NewFastJSONEncoder creates a new fast JSON encoder
func NewFastJSONEncoder(pool *BufferPool) *FastJSONEncoder {
	return &FastJSONEncoder{
		pool: pool,
	}
}

// EncodeOrder encodes an order to JSON
func (e *FastJSONEncoder) EncodeOrder(order *OrderData) []byte {
	w := NewJSONWriter(e.pool)
	defer w.Release()
	
	w.WriteObjectStart()
	
	w.WriteKey("id")
	w.WriteString(order.ID)
	w.WriteComma()
	
	w.WriteKey("symbol")
	w.WriteString(order.Symbol)
	w.WriteComma()
	
	w.WriteKey("side")
	w.WriteString(order.Side)
	w.WriteComma()
	
	w.WriteKey("type")
	w.WriteString(order.Type)
	w.WriteComma()
	
	w.WriteKey("price")
	w.WriteFloat(order.Price)
	w.WriteComma()
	
	w.WriteKey("quantity")
	w.WriteFloat(order.Quantity)
	w.WriteComma()
	
	w.WriteKey("timestamp")
	w.WriteInt(order.Timestamp)
	
	w.WriteObjectEnd()
	
	// Return a copy since we're releasing the buffer
	result := make([]byte, len(w.Bytes()))
	copy(result, w.Bytes())
	return result
}

// OrderData represents order data for encoding
type OrderData struct {
	ID        string
	Symbol    string
	Side      string
	Type      string
	Price     float64
	Quantity  float64
	Timestamp int64
}

// JSONObjectPool pools JSON objects for reuse
type JSONObjectPool struct {
	pool sync.Pool
}

// NewJSONObjectPool creates a new JSON object pool
func NewJSONObjectPool() *JSONObjectPool {
	return &JSONObjectPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &JSONObject{
					fields: make(map[string]interface{}, 16),
				}
			},
		},
	}
}

// Get retrieves a JSON object from the pool
func (p *JSONObjectPool) Get() *JSONObject {
	return p.pool.Get().(*JSONObject)
}

// Put returns a JSON object to the pool
func (p *JSONObjectPool) Put(obj *JSONObject) {
	obj.Reset()
	p.pool.Put(obj)
}

// JSONObject represents a reusable JSON object
type JSONObject struct {
	fields map[string]interface{}
}

// Reset clears the object for reuse
func (o *JSONObject) Reset() {
	for k := range o.fields {
		delete(o.fields, k)
	}
}

// Set sets a field value
func (o *JSONObject) Set(key string, value interface{}) {
	o.fields[key] = value
}

// MarshalJSON implements json.Marshaler
func (o *JSONObject) MarshalJSON() ([]byte, error) {
	return json.Marshal(o.fields)
}

// StreamingJSONWriter writes JSON directly to an output stream
type StreamingJSONWriter struct {
	w   *bytes.Buffer
	err error
}

// NewStreamingJSONWriter creates a new streaming JSON writer
func NewStreamingJSONWriter(w *bytes.Buffer) *StreamingJSONWriter {
	return &StreamingJSONWriter{
		w: w,
	}
}

// WriteObject writes an object with a callback
func (s *StreamingJSONWriter) WriteObject(fn func(*StreamingJSONWriter)) {
	s.writeByte('{')
	fn(s)
	s.writeByte('}')
}

// WriteArray writes an array with a callback
func (s *StreamingJSONWriter) WriteArray(fn func(*StreamingJSONWriter)) {
	s.writeByte('[')
	fn(s)
	s.writeByte(']')
}

// WriteField writes a field
func (s *StreamingJSONWriter) WriteField(key string, value interface{}) {
	s.writeString(key)
	s.writeByte(':')
	s.writeValue(value)
}

// writeValue writes a value of any type
func (s *StreamingJSONWriter) writeValue(v interface{}) {
	if s.err != nil {
		return
	}
	
	switch val := v.(type) {
	case string:
		s.writeString(val)
	case int:
		s.writeInt(int64(val))
	case int64:
		s.writeInt(val)
	case float64:
		s.writeFloat(val)
	case bool:
		s.writeBool(val)
	case nil:
		s.writeNull()
	default:
		// Fallback to standard JSON encoding
		data, err := json.Marshal(val)
		if err != nil {
			s.err = err
			return
		}
		s.w.Write(data)
	}
}

// Helper methods
func (s *StreamingJSONWriter) writeByte(b byte) {
	if s.err == nil {
		s.err = s.w.WriteByte(b)
	}
}

func (s *StreamingJSONWriter) writeString(str string) {
	if s.err != nil {
		return
	}
	s.writeByte('"')
	s.w.WriteString(str)
	s.writeByte('"')
}

func (s *StreamingJSONWriter) writeInt(i int64) {
	if s.err == nil {
		s.w.WriteString(strconv.FormatInt(i, 10))
	}
}

func (s *StreamingJSONWriter) writeFloat(f float64) {
	if s.err == nil {
		s.w.WriteString(strconv.FormatFloat(f, 'f', -1, 64))
	}
}

func (s *StreamingJSONWriter) writeBool(b bool) {
	if s.err == nil {
		if b {
			s.w.WriteString("true")
		} else {
			s.w.WriteString("false")
		}
	}
}

func (s *StreamingJSONWriter) writeNull() {
	if s.err == nil {
		s.w.WriteString("null")
	}
}

// Error returns any error that occurred during writing
func (s *StreamingJSONWriter) Error() error {
	return s.err
}