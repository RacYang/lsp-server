package room

// Broadcaster 将房间级事件下推到 gate/Hub，由 app 在装配时注入；nil 时忽略。
// msgID 与 client.v1 帧的 msg_id 一致，payload 为已序列化的 Protobuf Envelope 或其他约定载荷。
type Broadcaster interface {
	BroadcastRoom(roomID string, msgID uint16, payload []byte)
}
