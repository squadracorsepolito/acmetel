package message

type UDPPayload struct {
	embedded

	Data    []byte
	DataLen int
}

func NewUDPPayload(data []byte, dataLen int) *UDPPayload {
	return &UDPPayload{
		Data:    data,
		DataLen: dataLen,
	}
}
