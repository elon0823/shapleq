package integration_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/paust-team/paustq/broker"
	"github.com/paust-team/paustq/client"
	"github.com/paust-team/paustq/client/consumer"
	"github.com/paust-team/paustq/client/producer"
	paustqproto "github.com/paust-team/paustq/proto"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"testing"
	"time"
)

func SleepForBroker() {
	time.Sleep(500 * time.Millisecond)
}

func readFromFileLineBy(fileName string) (int, [][]byte) {
	f, err := os.Open(fileName)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	var records [][]byte
	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		data := []byte(scanner.Text())
		records = append(records, data)
		count++
	}
	return count, records
}

func contains(s [][]byte, e []byte) bool {
	for _, a := range s {
		if bytes.Compare(a, e) == 0 {
			return true
		}
	}
	return false
}


func TestClient_Connect(t *testing.T) {

	ip := "127.0.0.1"
	port := 9000
	host := fmt.Sprintf("%s:%d", ip, port)
	ctx := context.Background()
	topic := "test_topic1"

	// Start broker
	brokerInstance, err := broker.NewBroker(uint16(port))
	if err != nil {
		t.Error(err)
		return
	}

	defer brokerInstance.Clean()
	defer SleepForBroker()

	brokerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err := brokerInstance.Start(brokerCtx)
		if err != nil {
			t.Error("error on starting broker")
			return
		}
	}()

	defer SleepForBroker()

	// Start client
	c := client.NewStreamClient(host, paustqproto.SessionType_ADMIN)

	if err := c.Connect(ctx, topic); err != nil {
		t.Error("Error on connect. ", err)
		return
	}

	if err := c.Close(); err != nil {
		t.Error(err)
	}
}

func TestPubSub(t *testing.T) {

	ip := "127.0.0.1"
	port := 9001

	testRecordMap := map[string][][]byte{
		"topic1": {
			{'g', 'o', 'o', 'g', 'l', 'e'},
			{'p', 'a', 'u', 's', 't', 'q'},
			{'1', '2', '3', '4', '5', '6'}},
	}
	topic := "topic1"
	receivedRecordMap := make(map[string][][]byte)

	host := fmt.Sprintf("%s:%d", ip, port)
	ctx1 := context.Background()
	ctx2 := context.Background()

	// Start broker
	brokerInstance, err := broker.NewBroker(uint16(port))
	if err != nil {
		t.Error(err)
		return
	}

	defer brokerInstance.Clean()
	defer SleepForBroker()

	brokerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err := brokerInstance.Start(brokerCtx)
		if err != nil {
			t.Error("error on starting broker")
			return
		}
	}()

	defer SleepForBroker()

	// Start producer
	producerClient := producer.NewProducer(host)
	if producerClient.Connect(ctx1, topic) != nil {
		t.Error("Error on connect")
		return
	}

	for _, record := range testRecordMap[topic] {
		producerClient.Publish(ctx1, record)
	}

	producerClient.WaitAllPublishResponse()

	if err := producerClient.Close(); err != nil {
		t.Error(err)
	}

	// Start consumer
	consumerClient := consumer.NewConsumer(host)
	if consumerClient.Connect(ctx2, topic) != nil {
		t.Error("Error on connect")
		return
	}
	subscribeCh, err := consumerClient.Subscribe(ctx1, 0)
	if err != nil {
		t.Error(err)
		return
	}
	for response := range subscribeCh {
		if response.Error != nil {
			t.Error(response.Error)
		} else {
			receivedRecordMap[topic] = append(receivedRecordMap[topic], response.Data)
		}

		// break on reach end
		if response.LastOffset == response.Offset {
			break
		}
	}

	expectedResults := testRecordMap[topic]
	receivedResults := receivedRecordMap[topic]

	if len(expectedResults) != len(receivedResults) {
		t.Error("Length Mismatch - Expected records: ", len(expectedResults), ", Received records: ", len(receivedResults))
	}
	for i, record := range expectedResults {
		if bytes.Compare(receivedResults[i], record) != 0 {
			t.Error("Record is not same")
		}
	}

	if err := consumerClient.Close(); err != nil {
		t.Error(err)
	}
}

func TestPubsub_Chunk(t *testing.T) {

	ip := "127.0.0.1"
	port := 9002
	var chunkSize uint32 = 1024

	topic := "topic2"

	host := fmt.Sprintf("%s:%d", ip, port)
	ctx1 := context.Background()
	ctx2 := context.Background()

	// Start broker
	brokerInstance, err := broker.NewBroker(uint16(port))
	if err != nil {
		t.Error(err)
		return
	}

	defer brokerInstance.Clean()
	defer SleepForBroker()

	brokerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err := brokerInstance.Start(brokerCtx)
		if err != nil {
			t.Error("error on starting broker")
			return
		}
	}()

	defer SleepForBroker()

	// Start producer
	producerClient := producer.NewProducer(host).WithChunkSize(chunkSize)
	if producerClient.Connect(ctx1, topic) != nil {
		t.Error("Error on connect")
		return
	}

	data, err := ioutil.ReadFile("sample.txt")

	if err != nil {
		t.Error(err)
		return
	}

	producerClient.Publish(ctx1, data)
	producerClient.WaitAllPublishResponse()

	if err := producerClient.Close(); err != nil {
		t.Error(err)
	}

	expectedLen := len(data)
	// Start consumer

	consumerClient := consumer.NewConsumer(host)
	if consumerClient.Connect(ctx2, topic) != nil {
		t.Error("Error on connect")
		return
	}

	receivedLen := 0
	subscribeCh, err := consumerClient.Subscribe(ctx2, 0)
	if err != nil {
		t.Error(err)
		return
	}
	for response := range subscribeCh {
		if response.Error != nil {
			t.Error(response.Error)
		} else {
			receivedLen = len(response.Data)
		}

		// break on reach end
		if response.LastOffset == response.Offset {
			break
		}
	}

	if err := consumerClient.Close(); err != nil {
		t.Error(err)
	}

	if expectedLen != receivedLen {
		t.Error("Length Mismatch - Expected record length: ", expectedLen, ", Received record length: ", receivedLen)
	}
}

func TestMultiClient(t *testing.T) {

	ip := "127.0.0.1"
	port := 9003

	topic := "topic3"

	host := fmt.Sprintf("%s:%d", ip, port)

	brokerInstance, err := broker.NewBroker(uint16(port))
	if err != nil {
		t.Error(err)
		return
	}

	defer brokerInstance.Clean()
	defer SleepForBroker()

	brokerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err := brokerInstance.Start(brokerCtx)
		if err != nil {
			t.Error("error on starting broker")
			return
		}
	}()

	defer SleepForBroker()

	mu := sync.Mutex{}
	wg := sync.WaitGroup{}

	// Start producer
	var expectedSentCount int
	var sentRecords [][]byte

	runProducer := func(ctx context.Context, fileName string) {
		count, sendingRecords := readFromFileLineBy(fileName)
		expectedSentCount += count
		go func() {
			defer wg.Done()

			producerClient := producer.NewProducer(host)
			if producerClient.Connect(ctx, topic) != nil {
				t.Error("Error on connect")
				return
			}

			for _, record := range sendingRecords {
				producerClient.Publish(ctx, record)
			}

			mu.Lock()
			sentRecords = append(sentRecords, sendingRecords...)
			mu.Unlock()

			producerClient.WaitAllPublishResponse()

			if err := producerClient.Close(); err != nil {
				t.Error(err)
			}
			fmt.Println("publish done with file :", fileName, "sent total:", len(sendingRecords))
		}()
	}

	wg.Add(3)

	runProducer(context.Background(), "data1.txt")
	runProducer(context.Background(), "data2.txt")
	runProducer(context.Background(), "data3.txt")

	// Start consumer
	type ReceivedRecords [][]byte
	var totalReceivedRecords []ReceivedRecords

	runConsumer := func(ctx context.Context) {
		var receivedRecords ReceivedRecords

		wg.Add(1)
		go func() {
			defer wg.Done()

			consumerClient := consumer.NewConsumer(host)
			if consumerClient.Connect(ctx, topic) != nil {
				t.Error("Error on connect")
				return
			}

			receiveCount := 0
			subscribeCh, err := consumerClient.Subscribe(ctx, 0)
			if err != nil {
				t.Error(err)
				return
			}
			for response := range subscribeCh {
				if response.Error != nil {
					t.Error(response.Error)
				} else {

					receivedRecords = append(receivedRecords, response.Data)
					receiveCount++
					//fmt.Println("receiveCount", receiveCount)
					if expectedSentCount == receiveCount {
						fmt.Println("complete consumer. received :", receiveCount)
						break
					}
				}
			}

			totalReceivedRecords = append(totalReceivedRecords, receivedRecords)

			if err := consumerClient.Close(); err != nil {
				t.Error(err)
			}
		}()
	}

	runConsumer(context.Background())
	runConsumer(context.Background())

	wg.Wait()

	for _, receivedRecords := range totalReceivedRecords {
		if len(sentRecords) != len(receivedRecords) {
			t.Error("Length Mismatch - Expected records: ", len(sentRecords), ", Received records: ", len(receivedRecords))
		}

		for _, record := range receivedRecords {
			if !contains(sentRecords, record) {
				t.Error("Record is not exists: ", record)
			}
		}
	}
}

type MockZkHelper struct {
	TopicEndpoints 		map[string]string
}

func (zk *MockZkHelper) GetTopicEndpoint(topicName string) string {
	return zk.TopicEndpoints[topicName]
}

func TestMultiBroker(t *testing.T) {

	ip := "127.0.0.1"
	port1 := 9004
	port2 := 9005

	host1 := fmt.Sprintf("%s:%d", ip, port1)
	host2 := fmt.Sprintf("%s:%d", ip, port2)

	topicLocal := "topic_local"
	topicRemote := "topic_remote"

	testRecordMap := map[string][][]byte{
		topicLocal: {
			{'g', 'o', 'o', 'g', 'l', 'e'},
			{'p', 'a', 'u', 's', 't', 'q'},
			{'1', '2', '3', '4', '5', '6'}},
		topicRemote: {
			{'G', 'O', 'O', 'G', 'L', 'E'},
			{'P', 'A', 'U', 'S', 'T', 'Q'},
			{'R', 'E', 'M', 'O', 'T', 'E'}},
	}

	topicEndpoint := map[string]string {
		topicLocal: "localhost",
		topicRemote: host2,
	}

	mockZkHelper := &MockZkHelper{topicEndpoint}

	// Start broker 1
	brokerInstance1, err := broker.NewBroker(uint16(port1))
	if err != nil {
		t.Error(err)
		return
	}
	brokerInstance1 = brokerInstance1.WithInternalPort(1101).WithZkHelper(mockZkHelper)
	defer brokerInstance1.Clean()

	// Start broker 2
	brokerInstance2, err := broker.NewBroker(uint16(port2))
	if err != nil {
		t.Error(err)
		return
	}
	brokerInstance2 = brokerInstance2.WithInternalPort(1102).WithZkHelper(mockZkHelper)
	defer brokerInstance2.Clean()

	defer SleepForBroker()

	brokerCtx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	go func() {
		err := brokerInstance1.Start(brokerCtx1)
		if err != nil {
			t.Error("error on starting broker1")
			return
		}
	}()

	brokerCtx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	go func() {
		err := brokerInstance2.Start(brokerCtx2)
		if err != nil {
			t.Error("error on starting broker2")
			return
		}
	}()

	defer SleepForBroker()

	ctxProducer := context.Background()
	ctxConsumer := context.Background()

	// Start producer
	testProducer := func(topic string, host string) {
		producerClient := producer.NewProducer(host)
		if producerClient.Connect(ctxProducer, topic) != nil {
			t.Error("Error on connect to broker1")
			return
		}

		for _, record := range testRecordMap[topic] {
			producerClient.Publish(ctxProducer, record)
		}

		producerClient.WaitAllPublishResponse()

		if err := producerClient.Close(); err != nil {
			t.Error(err)
		}
	}

	testProducer(topicLocal, host1)
	testProducer(topicRemote, host2)

	// Start consumer
	// consumer requests data for topicLocal and topicRemote to host1 only
	testConsumer := func(topic string) {
		receivedRecordMap := make(map[string][][]byte)
		consumerClient := consumer.NewConsumer(host1)
		if consumerClient.Connect(ctxConsumer, topic) != nil {
			t.Error("Error on connect")
			return
		}
		subscribeCh, err := consumerClient.Subscribe(ctxConsumer, 0)
		if err != nil {
			t.Error(err)
			return
		}
		for response := range subscribeCh {
			if response.Error != nil {
				t.Error(response.Error)
			} else {
				receivedRecordMap[topic] = append(receivedRecordMap[topic], response.Data)
			}

			// break on reach end
			if response.LastOffset == response.Offset {
				break
			}
		}

		expectedResults := testRecordMap[topic]
		receivedResults := receivedRecordMap[topic]

		if len(expectedResults) != len(receivedResults) {
			t.Error("Length Mismatch - Expected records: ", len(expectedResults), ", Received records: ", len(receivedResults))
		}
		for i, record := range expectedResults {
			if bytes.Compare(receivedResults[i], record) != 0 {
				t.Error("Record is not same")
			}
		}

		if err := consumerClient.Close(); err != nil {
			t.Error(err)
		}
	}

	testConsumer(topicLocal)
	//testConsumer(topicRemote)
}


