package internal

type StageKind string

const (
	StageKindIngress      StageKind = "ingress"
	StageKindPreProcessor StageKind = "pre-processor"
	StageKindProcessor    StageKind = "processor"
)
