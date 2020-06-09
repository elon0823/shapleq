package rpc

import (
	"errors"
	"github.com/paust-team/paustq/broker/storage"
	"github.com/paust-team/paustq/message"
	"github.com/paust-team/paustq/pqerror"
	paustqproto "github.com/paust-team/paustq/proto"
	"github.com/paust-team/paustq/zookeeper"
)

type TopicRPCService interface {
	CreateTopic(*paustqproto.CreateTopicRequest) *paustqproto.CreateTopicResponse
	DeleteTopic(*paustqproto.DeleteTopicRequest) *paustqproto.DeleteTopicResponse
	ListTopic(*paustqproto.ListTopicRequest) *paustqproto.ListTopicResponse
	DescribeTopic(*paustqproto.DescribeTopicRequest) *paustqproto.DescribeTopicResponse
}

type topicRPCService struct {
	DB       *storage.QRocksDB
	zkClient *zookeeper.ZKClient
}

func NewTopicRPCService(db *storage.QRocksDB, zkClient *zookeeper.ZKClient) *topicRPCService {
	return &topicRPCService{db, zkClient}
}

func (s topicRPCService) CreateTopic(request *paustqproto.CreateTopicRequest) *paustqproto.CreateTopicResponse {

	if err := s.DB.PutTopicIfNotExists(request.Topic.TopicName, request.Topic.TopicMeta,
		request.Topic.NumPartitions, request.Topic.ReplicationFactor); err != nil {
		return message.NewCreateTopicResponseMsg(&pqerror.QRocksOperateError{ErrStr: err.Error()})
	}

	err := s.zkClient.AddTopic(request.Topic.TopicName)
	if err != nil {
		var e pqerror.ZKTargetAlreadyExistsError
		if errors.As(err, &e) {
			return message.NewCreateTopicResponseMsg(e)
		}
		return message.NewCreateTopicResponseMsg(&pqerror.ZKOperateError{ErrStr: err.Error()})
	}

	return message.NewCreateTopicResponseMsg(nil)
}

func (s topicRPCService) DeleteTopic(request *paustqproto.DeleteTopicRequest) *paustqproto.DeleteTopicResponse {

	if err := s.DB.DeleteTopic(request.TopicName); err != nil {
		return message.NewDeleteTopicResponseMsg(&pqerror.QRocksOperateError{ErrStr: err.Error()})
	}
	if err := s.zkClient.RemoveTopic(request.TopicName); err != nil {
		return message.NewDeleteTopicResponseMsg(&pqerror.ZKOperateError{ErrStr: err.Error()})
	}

	return message.NewDeleteTopicResponseMsg(nil)
}

func (s topicRPCService) ListTopic(_ *paustqproto.ListTopicRequest) *paustqproto.ListTopicResponse {

	topics, err := s.zkClient.GetTopics()
	if err != nil {
		return message.NewListTopicResponseMsg(nil, &pqerror.ZKOperateError{ErrStr: err.Error()})
	}

	return message.NewListTopicResponseMsg(topics, nil)
}

func (s topicRPCService) DescribeTopic(request *paustqproto.DescribeTopicRequest) *paustqproto.DescribeTopicResponse {

	// Temporary
	return message.NewDescribeTopicResponseMsg(request.TopicName, "", 0, 0, nil)
}