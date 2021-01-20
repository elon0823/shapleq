package service

import (
	"context"
	"errors"
	"github.com/golang/protobuf/proto"
	"github.com/paust-team/shapleq/broker/internals"
	"github.com/paust-team/shapleq/broker/service/rpc"
	"github.com/paust-team/shapleq/broker/storage"
	"github.com/paust-team/shapleq/message"
	shapleqproto "github.com/paust-team/shapleq/proto"
	"github.com/paust-team/shapleq/zookeeper"
	"runtime"
	"sync"
)

type RPCService struct {
	rpc.TopicRPCService
	rpc.PartitionRPCService
	rpc.ConfigRPCService
	rpc.GroupRPCService
	rpc.ConnectionRPCService
}

type TransactionService struct {
	rpcService *RPCService
}

func NewTransactionService(db *storage.QRocksDB, zkClient *zookeeper.ZKClient) *TransactionService {
	return &TransactionService{&RPCService{
		rpc.NewTopicRPCService(db, zkClient),
		rpc.NewPartitionRPCService(db, zkClient),
		rpc.NewConfigRPCService(),
		rpc.NewGroupRPCService(),
		rpc.NewConnectionRPCService(zkClient)},
	}
}

func (s *TransactionService) HandleEventStreams(brokerCtx context.Context, eventStreamCh <-chan internals.EventStream) <-chan error {
	errCh := make(chan error)
	var wg sync.WaitGroup
	go func() {
		defer func() {
			wg.Wait()
			close(errCh)
		}()
		for {
			select {
			case <-brokerCtx.Done():
				return
			case eventStream, ok := <-eventStreamCh:
				if ok {
					wg.Add(1)
					go func() {
						defer wg.Done()
						for {
							select {
							case msg := <-eventStream.MsgCh:
								if msg == nil {
									return
								}
								if err := s.handleMsg(msg, eventStream.Session); err != nil {
									errCh <- err
								}
							default:
							}
							runtime.Gosched()
						}
					}()
				}
				//default:
			}
			//runtime.Gosched()
		}
	}()

	return errCh
}

func (s *TransactionService) handleMsg(msg *message.QMessage, session *internals.Session) error {

	var resMsg proto.Message

	if reqMsg, err := msg.UnpackAs(&shapleqproto.CreateTopicRequest{}); err == nil {
		resMsg = s.rpcService.CreateTopic(reqMsg.(*shapleqproto.CreateTopicRequest))

	} else if reqMsg, err := msg.UnpackAs(&shapleqproto.DeleteTopicRequest{}); err == nil {
		resMsg = s.rpcService.DeleteTopic(reqMsg.(*shapleqproto.DeleteTopicRequest))

	} else if reqMsg, err := msg.UnpackAs(&shapleqproto.ListTopicRequest{}); err == nil {
		resMsg = s.rpcService.ListTopic(reqMsg.(*shapleqproto.ListTopicRequest))

	} else if reqMsg, err := msg.UnpackAs(&shapleqproto.DescribeTopicRequest{}); err == nil {
		resMsg = s.rpcService.DescribeTopic(reqMsg.(*shapleqproto.DescribeTopicRequest))

	} else if reqMsg, err := msg.UnpackAs(&shapleqproto.Ping{}); err == nil {
		resMsg = s.rpcService.Heartbeat(reqMsg.(*shapleqproto.Ping))

	} else if reqMsg, err := msg.UnpackAs(&shapleqproto.DiscoverBrokerRequest{}); err == nil {
		resMsg = s.rpcService.DiscoverBroker(reqMsg.(*shapleqproto.DiscoverBrokerRequest))
	} else {
		return errors.New("invalid message to handle")
	}

	qMsg, err := message.NewQMessageFromMsg(message.TRANSACTION, resMsg)
	if err != nil {
		return err
	}
	if err := session.Write(qMsg); err != nil {
		return err
	}
	return nil
}
