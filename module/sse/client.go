package sse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/antiloger/Gfy/core"
	ntypes "github.com/antiloger/Gfy/pkg/Ntypes"
)

type SSEClient struct {
	ID            string                 `json:"id"`
	OrgID         string                 `json:"author_id"`
	Meta          map[string]interface{} `json:"meta"`
	LastEventID   string                 `json:"last_event_id"`
	LastEventTime time.Time              `json:"last_event_type"`
	ConnectedTime time.Time              `json:"connected_time"`
	queue         chan *ntypes.SSENotifyPayload
	closed        int32
}

func NewSSEClient(id, orgID string, meta map[string]interface{}, queueSize int) *SSEClient {
	if queueSize <= 0 {
		queueSize = 10 // Default queue size
	}

	if meta == nil {
		meta = make(map[string]interface{})
	}

	return &SSEClient{
		ID:     id,
		OrgID:  orgID,
		Meta:   meta,
		queue:  make(chan *ntypes.SSENotifyPayload, queueSize),
		closed: 0, // Initially not closed
	}
}

func (c *SSEClient) UpdateLastEvent(eventID string, eventTime time.Time) {
	c.LastEventID = eventID
	c.LastEventTime = eventTime
}

func (c *SSEClient) SetConnectedTime(connectedTime time.Time) {
	c.ConnectedTime = connectedTime
}

func (c *SSEClient) GetID() (string, error) {
	if c.ID == "" {
		return "", errors.New("client ID is not set")
	}
	return c.ID, nil
}

func (c *SSEClient) SendMessageWithContext(ctx context.Context, payload *ntypes.SSENotifyPayload) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return fmt.Errorf("client %s is closed", c.ID)
	}
	// TODO: Add logger
	//
	//

	select {
	case c.queue <- payload:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *SSEClient) GetOrgID() (string, error) {
	if c.OrgID == "" {
		return "", errors.New("author ID is not set")
	}
	return c.OrgID, nil
}

// QueueCap returns the capacity of the queue
func (c *SSEClient) QueueCap() int {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0
	}
	return cap(c.queue)
}

// QueueLen returns the current number of messages in the queue
func (c *SSEClient) QueueLen() int {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0
	}
	return len(c.queue)
}

// GetQueueStats returns queue statistics
func (c *SSEClient) GetQueueStats() map[string]interface{} {
	return map[string]interface{}{
		"queue_length":    c.QueueLen(),
		"queue_capacity":  c.QueueCap(),
		"queue_full":      c.IsQueueFull(),
		"is_closed":       c.IsClosed(),
		"last_event_id":   c.LastEventID,
		"last_event_time": c.LastEventTime,
		"connected_time":  c.ConnectedTime,
	}
}

// IsQueueFull checks if the queue is full
func (c *SSEClient) IsQueueFull() bool {
	if atomic.LoadInt32(&c.closed) == 1 {
		return true
	}
	return len(c.queue) == cap(c.queue)
}

// IsClosed checks if the client is closed
func (c *SSEClient) IsClosed() bool {
	return atomic.LoadInt32(&c.closed) == 1
}

// Close safely closes the client and its queue
func (c *SSEClient) Close() {
	// Use atomic compare-and-swap to ensure we only close once
	if atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		close(c.queue)
	}
}

func (c *SSEClient) GetQueue() <-chan *ntypes.SSENotifyPayload {
	return c.queue
}

func (c *SSEClient) GetMeta() map[string]interface{} {
	return c.Meta
}

func (c *SSEClient) ClientMarshel() ([]byte, error) {
	data, err := json.Marshal(c)
	return data, err
}

func (c *SSEClient) ClientUnmarshel(data string) error {
	return json.Unmarshal([]byte(data), c)
}

func (s *SSEModule) addClient(client *SSEClient, ctx context.Context) error {
	cID, err := client.GetID()
	if err != nil {
		s.logger.Error("Failed to get client ID",
			core.Field{
				Key: "err", Value: err,
			})
		return err
	}
	authorID, err := client.GetOrgID()
	if err != nil {
		s.logger.Error("Failed to get Author ID",
			core.Field{
				Key: "err", Value: err,
			})
		return err
	}

	clientByte, err := client.ClientMarshel()
	if err != nil {
		s.logger.Error("Failed to marshal SSE client",
			core.Field{
				Key: "client_id", Value: cID,
			})
		return err
	}
	clientKey := fmt.Sprintf("sseclient:%s:%s", authorID, cID)

	s.rdsClient.Set(ctx, clientKey, clientByte, 24*time.Hour)
	return nil
}

func (s *SSEModule) getClient(clientID string, authorID string, ctx context.Context) (*SSEClient, error) {
	var c SSEClient
	clientStr, err := s.rdsClient.Get(ctx, fmt.Sprintf("sseclient:%s:%s", authorID, clientID)).Result()
	if err != nil {
		s.logger.Error("Failed to get SSE client",
			core.Field{
				Key: "err", Value: err,
			})
		return nil, err
	}

	err = c.ClientUnmarshel(clientStr)
	if err != nil {
		s.logger.Error("Failed to unmarshal SSE client",
			core.Field{
				Key: "err", Value: err,
			})
		return nil, err
	}

	// ✅ CRITICAL: Reinitialize the queue after unmarshaling
	c.queue = make(chan *ntypes.SSENotifyPayload, 100)
	c.closed = 0

	return &c, nil
}

func (s *SSEModule) removeClient(clientID string, ctx context.Context) error {
	return s.rdsClient.Del(ctx, fmt.Sprintf("sseclient:%s", clientID)).Err()
}

func (s *SSEModule) updateClient(client *SSEClient, ctx context.Context) error {
	cID, err := client.GetID()
	if err != nil {
		s.logger.Error("Failed to get client ID",
			core.Field{
				Key: "err", Value: err,
			})
		return err
	}
	authorID, err := client.GetOrgID()
	if err != nil {
		s.logger.Error("Failed to get Author ID",
			core.Field{
				Key: "err", Value: err,
			})
		return err
	}

	clientByte, err := client.ClientMarshel()
	if err != nil {
		s.logger.Error("Failed to marshal SSE client",
			core.Field{
				Key: "client_id", Value: cID,
			})
		return err
	}
	clientKey := fmt.Sprintf("sseclient:%s:%s", authorID, cID)

	s.rdsClient.Set(ctx, clientKey, clientByte, 24*time.Hour)
	return nil
}

type ClientManager struct {
	Clients map[string]map[string]*SSEClient // Map of clients indexed by org ID and client ID

	ACKChan         chan *ntypes.NotifyACKPayload
	OverflowACKChan chan *ntypes.NotifyACKPayload // Channel for overflow ACK payloads
	DeadACKChan     chan *ntypes.NotifyACKPayload // Channel for dead ACK payloads

	config       *ClientManagerConfig
	mu           sync.RWMutex
	shutdownMode int32 // 0 = normal, 1 = shutting down, 2 = closed
}

func NewClientManager(config *ClientManagerConfig) (*ClientManager, error) {
	if config == nil {
		config = DefaultClientManagerConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid client manager config: %w", err)
	}

	return &ClientManager{
		Clients:         make(map[string]map[string]*SSEClient),
		ACKChan:         make(chan *ntypes.NotifyACKPayload, config.ACKChannelBuffer),
		OverflowACKChan: make(chan *ntypes.NotifyACKPayload, config.OverflowChannelBuffer),
		DeadACKChan:     make(chan *ntypes.NotifyACKPayload, config.DeadChannelBuffer),
		config:          config,
		shutdownMode:    0, // Normal mode
	}, nil
}

func (cm *ClientManager) CanProcess() bool {
	return atomic.LoadInt32(&cm.shutdownMode) == 0
}

func (cm *ClientManager) addClientUnsafe(client *SSEClient) {
	if cm.Clients[client.OrgID] == nil {
		cm.Clients[client.OrgID] = make(map[string]*SSEClient)
	}
	cm.Clients[client.OrgID][client.ID] = client
}

func (cm *ClientManager) GetOrAddClient(client *SSEClient) (*SSEClient, error) {
	if !cm.CanProcess() {
		return nil, errors.New("client manager in shutdownMode")
	}

	if client == nil {
		return nil, errors.New("client cannot be nil")
	}

	cm.mu.RLock()
	if orgClients, ok := cm.Clients[client.OrgID]; ok && orgClients != nil {
		if existingClient := orgClients[client.ID]; existingClient != nil {
			cm.mu.RUnlock()
			return existingClient, nil
		}
	}
	cm.mu.RUnlock()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	if orgClients, ok := cm.Clients[client.OrgID]; ok && orgClients != nil {
		if existingClient := orgClients[client.ID]; existingClient != nil {
			return existingClient, nil
		}
	}

	cm.addClientUnsafe(client)

	return client, nil
}

func (cm *ClientManager) RemoveClient(clientID, orgID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Check if orgID exists
	orgClients, orgExists := cm.Clients[orgID]
	if !orgExists {
		return
	}

	// Check if client exists
	client, clientExists := orgClients[clientID]
	if !clientExists {
		return
	}

	// Safe to close and remove
	client.Close()
	delete(cm.Clients[orgID], clientID)

	// Clean up empty org map
	if len(cm.Clients[orgID]) == 0 {
		delete(cm.Clients, orgID)
	}
}

func (cm *ClientManager) GetClientExists(clientID, orgID string) (*SSEClient, bool) {
	if !cm.CanProcess() {
		return nil, false
	}
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if orgClients, orgExists := cm.Clients[orgID]; orgExists {
		if client, clientExists := orgClients[clientID]; clientExists {
			return client, true
		}
	}

	return nil, false
}

func (cm *ClientManager) SendMessageToClient(ctx context.Context, clientID, orgID string, payload *ntypes.SSENotifyPayload) error {
	if !cm.CanProcess() {
		return errors.New("client manager in shutdownMode")
	}
	// Get client while holding lock
	client, exists := cm.GetClientExists(clientID, orgID)
	if !exists {
		return fmt.Errorf("client %s not found in org %s", clientID, orgID)
	}

	// Send message without holding lock
	return client.SendMessageWithContext(ctx, payload)
}

func (cm *ClientManager) sendToOverflow(signal *ntypes.NotifyACKPayload) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	select {
	case cm.OverflowACKChan <- signal:
		return nil
	case <-ctx.Done():
		// Both channels full → retry with backoff
		return cm.sendWithRetry(signal)
	}
}

func (cm *ClientManager) sendToDeadLetter(signal *ntypes.NotifyACKPayload) error {
	select {
	case cm.DeadACKChan <- signal:
		log.Printf("ACK sent to dead letter queue: %s", signal.MsgID)
		return fmt.Errorf("ACK moved to dead letter queue after %d retries: %s", cm.config.MaxMessageRetry, signal.MsgID)
	default:
		// Even dead letter queue is full - this is critical
		log.Printf("CRITICAL: Dead letter queue full, ACK completely failed: %s", signal.MsgID)
		return fmt.Errorf("CRITICAL: all ACK channels full, message lost: %s", signal.MsgID)
	}
}

func (cm *ClientManager) sendWithRetry(signal *ntypes.NotifyACKPayload) error {
	for attempt := 0; attempt < cm.config.MaxMessageRetry; attempt++ {
		signal.RetryCount = attempt + 1

		if attempt > 0 {
			delay := cm.config.MessageRetryDelay * time.Duration(1<<(attempt-1))
			time.Sleep(delay)
		}

		// Try both channels
		select {
		case cm.ACKChan <- signal:
			log.Printf("ACK sent after %d retries: %s", attempt+1, signal.MsgID)
			return nil
		default:
			select {
			case cm.OverflowACKChan <- signal:
				log.Printf("ACK sent to overflow after %d retries: %s", attempt+1, signal.MsgID)
				return nil
			default:
				continue // Retry
			}
		}
	}

	// All retries failed - send to dead letter queue
	return cm.sendToDeadLetter(signal)
}

func (cm *ClientManager) SendAKGSignal(signal *ntypes.NotifyACKPayload) error {
	// Try primary channel first (non-blocking)
	select {
	case cm.ACKChan <- signal:
		return nil
	default:
		// Primary full → try overflow channel with timeout
		return cm.sendToOverflow(signal)
	}
}

type ClientManagerConfig struct {
	Workers               int           `json:"workers"`
	ACKWorkers            int           `json:"ack_workers"`
	ACKChannelBuffer      int           `json:"ack_channel_buffer"`
	OverflowChannelBuffer int           `json:"overflow_channel_buffer"`
	DeadChannelBuffer     int           `json:"dead_channel_buffer"`
	ClientTimeout         time.Duration `json:"client_timeout"`
	MessageTimeout        time.Duration `json:"message_timeout"`
	MaxMessageRetry       int           `json:"message_retry"`
	MessageRetryDelay     time.Duration `json:"message_retry_delay"`
	EnableMetrics         bool          `json:"enable_metrics"`
	ClientQueueSize       int           `json:"client_queue_size"`
}

func DefaultClientManagerConfig() *ClientManagerConfig {
	return &ClientManagerConfig{
		Workers:               4,
		ACKWorkers:            2,
		ACKChannelBuffer:      500,
		OverflowChannelBuffer: 250,
		DeadChannelBuffer:     100,
		ClientTimeout:         30 * time.Second,
		MessageTimeout:        30 * time.Second,
		MessageRetryDelay:     10 * time.Second,
		EnableMetrics:         true,
		MaxMessageRetry:       3,
		ClientQueueSize:       10,
	}
}

func (c *ClientManagerConfig) Validate() error {
	if c.Workers <= 0 {
		return fmt.Errorf("workers must be greater than 0, got: %d", c.Workers)
	}

	if c.ACKWorkers <= 0 {
		return fmt.Errorf("ack_workers must be greater than 0, got: %d", c.ACKWorkers)
	}

	if c.ACKChannelBuffer < 0 {
		return fmt.Errorf("ack_channel_buffer must be non-negative, got: %d", c.ACKChannelBuffer)
	}

	if c.MessageTimeout.Seconds() <= 0 {
		return fmt.Errorf("message_timeout must be greater than 0 seconds, got: %v", c.MessageTimeout.Seconds())
	}

	return nil
}

func (cm *ClientManager) Close() error {
	if !atomic.CompareAndSwapInt32(&cm.shutdownMode, 0, 2) {
		// If already shutting down (1), force to closed (2)
		atomic.StoreInt32(&cm.shutdownMode, 2)
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Close all clients immediately
	for orgID, orgClients := range cm.Clients {
		for clientID, client := range orgClients {
			client.Close()
			delete(orgClients, clientID)
		}
		delete(cm.Clients, orgID)
	}

	// Close channels
	close(cm.ACKChan)
	close(cm.OverflowACKChan)
	close(cm.DeadACKChan)

	log.Printf("ClientManager: Closed immediately")
	return nil
}
