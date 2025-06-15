***REMOVED***

# Distributed Data Sharing System (applied to Temperature) 🌦️ $\to$ ⚙️ $\to$ 📈


## Project Description 🗃️

This project implements a **fully decentralized distributed system** where multiple types of nodes collaborate to collect, verify, and use data across different sites.  

The system models a real-world scenario where devices collect sensor data, verify its accuracy, and produce predictions, while ensuring correct coordination and consistency across geographically separate nodes.

For educational purpose, it is applied to temperature data: sensors get temperature, and users predict the next day weather based on the 15 days past data. 
All system's nodes will work on their own copy of this dataset (the 15 past data, *15 past days as working with temperature*).

The core idea is that **sensors might give erroneous data, perturbating the users**. Thus, verifier systems are integrated to update, slowly, each data point one after the other.
The goal is to observe the impact of verifier parameters on user behaviors.

Each node maintains a **local replica** of the shared dataset (past 15 days of temperature readings) and participates in maintaining **consistency** between replicas using **locks** and **logical clocks**.


---

## Usage 🔌

Build: 
```bash
go build main.go
```

Then, to create a network: 

```bash
./network_ring.sh A:-node_type,sensor \ B:-node_type,verifier \ C:-node_type,sensor \ D:-node_type,user_exp \ E:-node_type,sensor \ F:-node_type,user_linear \ G:-node_type,verifie
```

The web views will be on http://localhost:8085/ and http://localhost:8083/ (one for each user node).


The `main` program takes the arguments:

| Argument     | Meaning                                                                 |
|--------------|-------------------------------------------------------------------------|
| `-node_type` | Type of node: sensor, verifier, user_linear, user_exp (default: sensor) |
| `-node_name` | Name of the node (default: "Sensor 1")                                  |

To create a unidirectional ring network, use the `network_ring_unidirectional.sh` script.

For a bidirectional ring network,use the `network_ring.sh`.

Below is a **bi**directional ring network (`network_ring.sh`):

![ring network demo image](docs/ring_network.png)



## Key Features 💡

- **Decentralized Architecture**:  
  No centralized database. All nodes maintain and update their own local replica.

- **Replica Consistency**:  
  Nodes coordinate using a **distributed queue with logical clocks** to serialize all updates.

- **Shared Data Management**:  
  Nodes work on a **sliding window** of the latest 15 days of temperature readings.

- **P2P Communication**:  
  Direct messaging between nodes for update propagation and coordination.

- **Flexible Role Execution**:  
  A single program can run in different node modes (`sensor`, `verifier`, or `user`) based on configuration at launch.


### Pear Discovery

Pear discovery works as so:

- The node whose id is `0_control` (the first control node), no matter the application it is related to, will be the initiator and will send a `pear_discovery` message to all other control nodes (*after a wait of 1 second, to make sure all nodes did start*)
- all control nodes will answer to `0_control` (directly, and only) with their own names
- after another wait of 1 second, the initiator will accept all received names as definitive, as close the pear discovery. It will thus propagate all received node names to all nodes, so that every control layers know who is in the network (and thus how many).
- The initiator will then start its application layer.
- The other nodes, on startup, wait for 2 seconds (equivalent to initiator's 2 one second waits) before starting their application layer.

---

## Scenario Example 🎉

1. **Sensor** generates a new reading (e.g., 25°C).
2. It broadcasts the reading to all nodes, including verifiers and users, which will store it in their local data stores.
3. **Verifiers** receive the reading and ask for a lock to verify the data. They send a request to all other verifiers.
4. Once all verifiers grant access, the requesting verifier processes the data (e.g., checks for anomalies) and updates the local data store.
5. It then releases the lock and sends the verified data to all nodes, including users.
6. **Users** receive the verified data and update their local data stores.
7. **Users** read all local data to predict the temperature, without needing to lock.

---

### Example by sequence diagrams

#### Operations flow at system startup

<details open>
  <summary>System startup diagram</summary>

```mermaid
sequenceDiagram
    participant Main as Main Application
    participant CN as Child Node (Sensor/Verifier/User)
    participant CL as Control Layer
    
    Main->>Main: Parse command line arguments
    Main->>Main: Validate node_type
 	
    alt node_type = "sensor"
        Main->>CN: NewSensorNode(id, interval, errorRate)
    else node_type = "verifier"
        Main->>CN: NewVerifierNode(id, processingCapacity)
    else node_type = "user"
        Main->>CN: NewUserNode(id, model)
    end   
	
    Main->>CL: NewControlLayer(id + "_control", childNode)
    
    Main->>CL: Start()
    activate CL
    CL->>CL: PearDiscovery()
    CL->>CN: SetControlLayer(controlLayer)
    CL->>CN: Start()
    activate CN

    
    
    CL->>CL: Begin message handling loop
    CN->>CN: Begin node-specific operations
    deactivate CN
    deactivate CL
    
```
	
</details>

#### Message handling inner flow

<details open>
  <summary>A message arrives at the control layer, which needs to check the destination (is it for a control operation, or for the application layer)</summary>

```mermaid
sequenceDiagram
    participant Other as Other Nodes
    participant CL as Control Layer
    participant CN as Child Node (Verifier/User)
    
    Other->>CL: Message arrives via stdin
    activate CL
    
    CL->>CL: Extract message ID
    
    alt message ID already seen
        CL-->>CL: Discard message (prevent loops)
    else message ID is new
        CL->>CL: Remember this ID
        CL->>CL: Update clock
        
        alt msg_destination = "applications"
            CL->>CL: Parse message details
            
            alt msg_type = "new_reading"
                CL->>CN: Send to application layer
                activate CN
                CN->>CN: Process reading according to node type
                deactivate CN
                
                CL->>Other: Propagate message to other nodes
            end
        else msg_destination = "control"
            CL->>CL: Handle control messages & propagate
        end
    end
    
    deactivate CL
```
	
</details>

#### Message handling outer flow


<details open>
  <summary>Message flow between controllers and nodes. An example with one node of each type</summary>

```mermaid
sequenceDiagram
    participant Main as Main Application
    participant SN as SensorNode
    participant SCL as Sensor ControlLayer
    participant VCL as Verifier ControlLayer
    participant VN as VerifierNode
    participant UCL as User ControlLayer
    participant UN as UserNode

    Main->>SN: Start()
    Main->>SCL: Start()
    
    activate SN
    Note over SN: Every readInterval
    SN->>SN: generateReading()
    SN->>SN: Increment clock
    SN->>SN: GenerateUniqueMessageID()
    SN->>SCL: SendMsgFromApplication(reading)
    activate SCL
    SCL->>SCL: RememberThisID()
    SCL->>VCL: Propagate message
    activate VCL
    Note over VCL: Check if message ID seen before
    VCL->>VCL: RememberThisID()
    VCL->>VCL: Update clock
    VCL->>VN: Send to application layer
    deactivate VCL
    
    VCL->>UCL: Propagate message to other controls
    activate UCL
    Note over UCL: Check if message ID seen before
    UCL->>UCL: RememberThisID()
    UCL->>UCL: Update clock
    UCL->>UN: Send to application layer
    deactivate UCL
    deactivate SCL
    deactivate SN
```
	
</details>

---


## Data Flow 🌊

Below is a flowchart representing the broadcasting of a sensor data, then verification of this data with request & release.
From reading the chart, it can be seen that:

- only verifier nodes and sensors need to send data
- only verifier nodes need to request & release data, and only between them.
- users only receive and update their local data replica.

<details open>
  <summary>Diagram</summary>
	
```mermaid
sequenceDiagram
    participant S as Sensor Node
    participant V1 as Verifier Node 1
    participant V2 as Verifier Node 2
    participant V3 as Verifier Node 3
    participant U as User Nodes
    
    S->>S: Generate temperature reading
    S->>+V1: Broadcast reading
    S->>+V2: Broadcast reading
    Note over V1,V2: Store reading in local datastores

    S->>+U: Broadcast reading
    Note over U: Store reading in local datastore
    
    
    Note over V1,V3: Verifier 1 discovers an unverified reading
    
    V1->>V2: Lock Request (itemID)
    V1->>V3: Lock Request (itemID)
    
    V2->>V1: Lock Reply (granted: true)
    V3->>V1: Lock Reply (granted: true)
    
    Note over V1: All nodes granted lock
    
    V1->>V2: Lock Acquired (itemID)
    Note over V2: Mark item as locked
    V1->>V3: Lock Acquired (itemID)
    Note over V3: Mark item as locked
    
    Note over V1: Process item (2-second wait)
    
    
    Note over V1: Mark item as verified
    
    V1->>V2: Lock Release & Verified Value (itemID, value)
    V1->>V3: Lock Release & Verified Value (itemID, value)
    
    Note over V2,V3: Update verified status & clear locks)
    V1->>U: Lock Release & Verified Value (itemID, value)
    Note over U: Update verified status
```

</details>

## Class Diagram 🔬

Below is a proposition of class diagram.

- Senrors, Verifiers, Users are all Nodes, thus share a basic structure (Node class), 
  and has their own DataStore.
- Sensors produce Readings
- clocks are represented via integers.


<details open>
  <summary>Class Diagram</summary>
	
```mermaid
classDiagram
    class Node {
        <<interface>>
        +Start() error
        +ID() string
        +Type() string
        +HandleMessage(chan string)
        +GetName() string
        +SetControlLayer(*ControlLayer) error
    }

    class BaseNode {
        -id string
        -nodeType string
        -isRunning bool
        -clock int
        -ctrlLayer *ControlLayer
        -nbMsgSent int
        +ID() string
        +Type() string
        +GetName() string
        +SetControlLayer(*ControlLayer) error
        +GenerateUniqueMessageID() string
        +NbMsgSent() int
    }

    class SensorNode {
        -readInterval time.Duration
        -errorRate float64
        +Start() error
        +ID() string
        +Type() string
        +HandleMessage(chan string)
        -generateReading() Reading
    }

   
    class VerifierNode {
        -processingCapacity: int
        -threshold: float64
        -verificationLocks: map[string]bool
        -lockRequests: map[string]map[string]int
        -lockedItems: map[string]string
        -lockReplies: map[string]map[string]bool
        -otherVerifiers: map[string]bool
        -nbOfVerifiers: int
        -recentReadings: map[string][]models.Reading
        -processingItems: map[string]bool
        -pendingItems: []models.Reading
        -verifiedItemIDs: map[string][]string
        -mutex: sync.Mutex
        +NewVerifierNode(id string, capacity int, threshold float64): *VerifierNode
        +Start(): error
        +HandleMessage(channel chan string)
        +removeAllExistenceOfReading(readingID string)
        +SendMessage(msg string)
        +CheckUnverifiedItems()
        +chooseReadingToVerify(): models.Reading
        +findUnverifiedReadings()
        +requestLock(reading models.Reading)
        +handleLockRequest(msg string)
        +handleLockReply(msg string)
        +cancelLockRequest(itemID string)
        +acquiredFullLockOnItem(itemID string)
        +handleLockAcquired(msg string)
        +processItem(itemID string)
        +getItemReadingValue(itemID string): float32
        +clampToAcceptableRange(value float32): float32
        +markItemAsVerified(itemID string, value float32, verifier string)
        +releaseLock(itemID string)
        +handleLockRelease(msg string)
        +getValueFromReadingID(itemID string): (float32, error)
        +getReadingIndexFromSender_ID(senderID string, itemID string): (int, error)
        +isItemInReadings(itemID string): (bool, error)
    } 

    class UserNode {
        -func predictionFunc(values []float32, decay float32) float32
        -float32 decayFactor
        -float32* lastPrediction
        -map[string][]models.Reading recentReadings
        -map[string][]float32 recentPredictions
        -map[string][]string verifiedItemIDs
        -sync.Mutex mutex
        +Start() error
        +HandleMessage(channel chan string)
        -handleLockRelease(msg string)
        -processDatabse()
        -printDatabase()
    }
	
    class ControlLayer {
        -sync.RWMutex mu
        -string id
        -string nodeType
        -bool isRunning
        -int clock
        -Node child
        -uint64 nbMsgSent
        -MIDWatcher* IDWatcher
        -chan string channel_to_application
        -int nbOfKnownSites
        -[]string knownSiteNames
        -[]string knownVerifierNames
        -bool sentDiscoveryMessage
        -bool pearDiscoverySealed
        +GetName() string
        +GenerateUniqueMessageID() string
        +Start() error
        +HandleMessage(msg string) error
        +AddNewMessageId(sender_name string, MID_str string)
        +SendApplicationMsg(msg string) error
        +SendControlMsg(msg_content string, msg_content_type string, msg_type string, destination string, fixed_id string, sender_name_source string) error
        +SendMsg(msg string, through_channelArgs ...bool)
        +SendPearDiscovery()
        +ClosePearDiscovery()
        +SawThatMessageBefore(msg string) bool
    }
	
    class Reading {
	+ReadingID string
        +Temperature float32
	+Clock int
        +SensorID string
        +IsVerified bool
	+VerifierID string
    }
    
        class MID {
        +int V
        +LessThan(other MID) bool
        +LessThanOrEqual(other MID) bool
        +Equal(other MID) bool
        +IsAdjacentAbove(other MID) bool
        +IsAdjacentBelow(other MID) bool
        +String() string
    }
    
    class MIDPair {
        +MID Lower
        +MID Upper
    }
    
    class MIDPairIntervals {
        +[]MIDPair Intervals
        +AddPair(pair MIDPair) void
        +Contains(clock MID) bool
        +AddMID(clk MID) void
    }
    
    class MIDWatcher {
        +map[string]MIDPairIntervals site_clock
        +ContainsMID(nodeID string, clk MID) bool
        +AddMIDToNode(nodeID string, clk MID) void
        +String() string
    }
    
    %% Relationships
    
    MIDPair "1" --* "2" MID : contains
    MIDPairIntervals "1" --* "0..*" MIDPair : contains
    MIDWatcher "1" --* "0..*" MIDPairIntervals : contains per node
    
    note for MID "Represents a Message ID"
    note for MIDPair "Defines bounds for message intervals"
    note for MIDPairIntervals "Manages collections of intervals"
    note for MIDWatcher "Tracks message IDs across multiple nodes"

    Node <|.. BaseNode : implements
    BaseNode <|-- SensorNode : extends
    BaseNode <|-- VerifierNode : extends
    BaseNode <|-- UserNode : extends
    
    SensorNode *-- Reading
    VerifierNode o-- Reading 
    UserNode o-- Reading
    
    ControlLayer *-- MIDWatcher : has
    
    VerifierNode "1" --o "1" ControlLayer
    UserNode "1" --o "1" ControlLayer
    SensorNode "1" --o "1" ControlLayer 
    
```

</details>
