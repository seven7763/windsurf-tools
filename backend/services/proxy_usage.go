package services

// ExtractMitmMetadata 从发往服务端的 Protobuf body 中提取请求的模型名和估算的 Prompt Tokens
func ExtractMitmMetadata(body []byte) (model string, promptTokens int) {
	raw, _ := decompressBody(body)
	fields := parseProtobuf(raw)

	for _, f := range fields {
		switch f.FieldNum {
		case 2:
			// F2: System Prompt
			if f.WireType == 2 {
				promptTokens += estimateTokens(string(f.Bytes))
			}
		case 3:
			// F3: Chat Messages (repeated)
			if f.WireType == 2 {
				subFields := parseProtobuf(f.Bytes)
				for _, sf := range subFields {
					// 内部的 F3 是内容
					if sf.FieldNum == 3 && sf.WireType == 2 {
						promptTokens += estimateTokens(string(sf.Bytes))
					}
				}
			}
		case 21:
			// F21: Model enum
			if f.WireType == 2 {
				model = string(f.Bytes)
			}
		}
	}
	return
}
