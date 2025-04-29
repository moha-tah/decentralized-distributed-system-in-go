# Distributed Data Sharing System (applied to Temperature) 🌦️ $\to$ ⚙️ $\to$ 📈

## Project Description 🗃️

This project implements a **fully decentralized distributed system** where multiple types of nodes collaborate to collect, verify, and use data across different sites.  

The system models a real-world scenario where devices collect sensor data, verify its accuracy, and produce predictions, while ensuring correct coordination and consistency across geographically separate nodes.

For educational purpose, it is applied to temperature data: sensors get temperature, and users predict the next day weather based on the 15 days past data. 
All system's nodes will work on their own copy o fthis dataset (the 15 past data, *15 past days as working with temperature*).

The core idea is that **sensors might give erroneous data, perturbating the users**. Thus, verifier systems are integrated to update, slowly, each data point one after the other.
The goal is to observe the impact of verifier parameters on user behaviors.

Each node maintains a **local replica** of the shared dataset (past 15 days of temperature readings) and participates in maintaining **consistency** between replicas using a **distributed mutual exclusion algorithm** based on **Lamport timestamps**.


---

## Usage 🔌

Build: 
```bash
go build main.go
```

Then, to create a connection between a Sensor (which send a message every two seconds) and 
a Verifier (which prints the received message): 

```bash
./main -node_type sensor | ./main -node_type verifier
```

which produces:
```bash
 + [Verifier_1 163913] main     : Verifier_1 received </=sender_name=Sensor 1/=clk=1>
 + [Verifier_1 163913] main     : Verifier_1 received </=sender_name=Sensor 1/=clk=2>
 + [Verifier_1 163913] main     : Verifier_1 received </=sender_name=Sensor 1/=clk=3>
```

The `main` program takes the arguments:

| Argument     | Meaning                                                |
|--------------|--------------------------------------------------------|
| `-node_type` | Type of node: sensor, verifier, user (default: sensor) |
| `-node_name` | Name of the node (default: "Sensor 1")                 |

To create a unidirectional ring network, use the `network_ring_unidirectional.sh` script.

For a bidirectional ring network,use the `network_ring.sh`.

Below is a **bi**directional ring network (`network_ring.sh`):

![ring network demo image](docs/ring_network.png)

### Examples

### First example with a bidirectional ring 

Two nodes are present: a sensor and a verifier. Below is a full execution (stopped after a few seconds), which will then be explained:

```bash
❯ ./network_ring.sh \
  A:-node_type,sensor \
  B:-node_type,verifier


✅ Launched 2 nodes: A B
   (hit Ctrl+C to stop & clean up)


 + [control_laye 30034] Start()  : Starting control layer  (1_control)

 + [node_verifie 30034] Start()  : Starting verifier node verifier (1)

 + [control_laye 30032] Start()  : Starting control layer  (0_control)

 + [node_sensor. 30032] Start()  : Starting sensor node 0

 * [ (0_control) 30032] msg_send : émission de /=destination=applications/=clk=1/=content_type=sensor_reading/=content_value=20.137077/=id=sensor_0_0/=type=new_reading/=sender_name=sensor (0)/=sender_type=sensor

/=destination=applications/=clk=1/=content_type=sensor_reading/=content_value=20.137077/=id=sensor_0_0/=type=new_reading/=sender_name=sensor (0)/=sender_type=sensor
 + [HandleMessag 30034]  (1_cont :  (1_control) received the new reading <20.137077>

 * [ (1_control) 30034] msg_send : émission de /=destination=applications/=clk=1/=content_type=sensor_reading/=content_value=20.137077/=id=sensor_0_0/=type=new_reading/=sender_name=sensor (0)/=sender_type=sensor

/=destination=applications/=clk=1/=content_type=sensor_reading/=content_value=20.137077/=id=sensor_0_0/=type=new_reading/=sender_name=sensor (0)/=sender_type=sensor
 + [HandleMessag 30034] verifier : verifier (1) received the new reading <20.137077>

 * [ (0_control) 30032] msg_send : émission de /=clk=2/=content_type=sensor_reading/=content_value=-78.124306/=id=sensor_0_1/=type=new_reading/=sender_name=sensor (0)/=sender_type=sensor/=destination=applications

/=clk=2/=content_type=sensor_reading/=content_value=-78.124306/=id=sensor_0_1/=type=new_reading/=sender_name=sensor (0)/=sender_type=sensor/=destination=applications
 + [HandleMessag 30034] verifier : verifier (1) received the new reading <-78.124306>

 + [HandleMessag 30034]  (1_cont :  (1_control) received the new reading <-78.124306>

 * [ (1_control) 30034] msg_send : émission de /=clk=2/=content_type=sensor_reading/=content_value=-78.124306/=id=sensor_0_1/=type=new_reading/=sender_name=sensor (0)/=sender_type=sensor/=destination=applications

/=clk=2/=content_type=sensor_reading/=content_value=-78.124306/=id=sensor_0_1/=type=new_reading/=sender_name=sensor (0)/=sender_type=sensor/=destination=applications
 * [ (0_control) 30032] msg_send : émission de /=destination=applications/=clk=3/=content_type=sensor_reading/=content_value=20.628366/=id=sensor_0_2/=type=new_reading/=sender_name=sensor (0)/=sender_type=sensor

/=destination=applications/=clk=3/=content_type=sensor_reading/=content_value=20.628366/=id=sensor_0_2/=type=new_reading/=sender_name=sensor (0)/=sender_type=sensor
 + [HandleMessag 30034]  (1_cont :  (1_control) received the new reading <20.628366>

 * [ (1_control) 30034] msg_send : émission de /=destination=applications/=clk=3/=content_type=sensor_reading/=content_value=20.628366/=id=sensor_0_2/=type=new_reading/=sender_name=sensor (0)/=sender_type=sensor

/=destination=applications/=clk=3/=content_type=sensor_reading/=content_value=20.628366/=id=sensor_0_2/=type=new_reading/=sender_name=sensor (0)/=sender_type=sensor
 + [HandleMessag 30034] verifier : verifier (1) received the new reading <20.628366>

 * [ (0_control) 30032] msg_send : émission de /=content_value=-56.102753/=id=sensor_0_3/=type=new_reading/=sender_name=sensor (0)/=sender_type=sensor/=destination=applications/=clk=4/=content_type=sensor_reading

/=content_value=-56.102753/=id=sensor_0_3/=type=new_reading/=sender_name=sensor (0)/=sender_type=sensor/=destination=applications/=clk=4/=content_type=sensor_reading
 + [HandleMessag 30034]  (1_cont :  (1_control) received the new reading <-56.102753>

 * [ (1_control) 30034] msg_send : émission de /=content_value=-56.102753/=id=sensor_0_3/=type=new_reading/=sender_name=sensor (0)/=sender_type=sensor/=destination=applications/=clk=4/=content_type=sensor_reading

 + [HandleMessag 30034] verifier : verifier (1) received the new reading <-56.102753>
```

Let's analyze this.

First, each control layer is activated, and starts the associated application layer (sensor and verifier):

```
 + [control_laye 30034] Start()  : Starting control layer  (1_control)
 + [node_verifie 30034] Start()  : Starting verifier node verifier (1)
 + [control_laye 30032] Start()  : Starting control layer  (0_control)
 + [node_sensor. 30032] Start()  : Starting sensor node 0
```

Then, the sensor reads a temperature and asks to its control layer to send the message (the temperature) to all nodes. What is written is the control layer warning that it will send a message, and then the message:

```
 * [ (0_control) 30032] msg_send : émission de /=destination=applications/=clk=1/=content_type=sensor_reading/=content_value=20.137077/=id=sensor_0_0/=type=new_reading/=sender_name=sensor (0)/=sender_type=sensor

/=destination=applications/=clk=1/=content_type=sensor_reading/=content_value=20.137077/=id=sensor_0_0/=type=new_reading/=sender_name=sensor (0)/=sender_type=sensor
```

As the message is sent, the control layer of the second node (the verifier's control layer) receives this message :

```
 + [HandleMessag 30034]  (1_cont :  (1_control) received the new reading <20.137077>

```

and as this control layer never saw this message before, it then re-emit this message to others : (*even if here, it has no other neighbor*)

```
 * [ (1_control) 30034] msg_send : émission de /=destination=applications/=clk=1/=content_type=sensor_reading/=content_value=20.137077/=id=sensor_0_0/=type=new_reading/=sender_name=sensor (0)/=sender_type=sensor
```

The sensor's control layer doesn't process this re-emitted message, at it has saved the sent message's id, so it knows it already processed this message.

The verifier's control layer also gave the message to its application layer,  the verifier :

```
 + [HandleMessag 30034] verifier : verifier (1) received the new reading <20.137077>
```

Which complete the cycle! Then, the sensor's control layer receives a new message from its application level, thus emits the new temperature (new message from `sender_name = sensor (0)` with a clock of 2). And the cycle continues.

### Example by sequence diagrams

#### Operations flow at system startup

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
        Main->>CN: NewVerifierNode(id, processingCapacity, threshold)
    else node_type = "user"
        Main->>CN: NewUserNode(id, model, predictionWindow)
    end
    
    Main->>CL: NewControlLayer(id + "_control", childNode)
    
    Main->>CL: Start()
    activate CL
    CL->>CN: SetControlLayer(controlLayer)
    CL->>CN: Start()
    activate CN
    
    CL->>CL: Begin message handling loop
    CN->>CN: Begin node-specific operations
    deactivate CN
    deactivate CL
    
    Main->>Main: select{} (block forever)
```

#### Message handling inner flow

A message arrives at the control layer, which needs to check the destination (is it for a control operation, or for the application layer) :

```mermaid
sequenceDiagram
    participant CL as Control Layer
    participant CN as Child Node (Verifier/User)
    participant Other as Other Nodes
    
    Other->>CL: Message arrives via stdin
    activate CL
    
    CL->>CL: Extract message ID
    
    alt message ID already seen
        CL-->>CL: Discard message (prevent loops)
    else message ID is new
        CL->>CL: AddNewMessageId(msg_id)
        CL->>CL: Update Lamport clock
        
        alt msg_destination = "applications"
            CL->>CL: Parse message details
            
            alt msg_type = "new_reading"
                CL->>CN: Send to channel_to_application
                activate CN
                CN->>CN: Process reading according to node type
                deactivate CN
                
                CL->>Other: Propagate message to other nodes
            end
        else msg_destination = "control"
            CL->>CL: Handle control messages
        end
    end
    
    deactivate CL
```

#### Message handling outer flow

Message flow between controllers and nodes. An example with one node of each type :

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
    SCL->>SCL: AddNewMessageId()
    SCL->>VCL: Msg_send(reading message)
    activate VCL
    Note over VCL: Check if message ID seen before
    VCL->>VCL: AddNewMessageId()
    VCL->>VCL: Update Lamport clock
    VCL->>VN: Send to channel_to_application
    deactivate VCL
    
    VCL->>UCL: Propagate message
    activate UCL
    Note over UCL: Check if message ID seen before
    UCL->>UCL: AddNewMessageId()
    UCL->>UCL: Update Lamport clock
    UCL->>UN: Send to channel_to_application
    deactivate UCL
    deactivate SCL
    deactivate SN
```

---

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

---

## Technical Highlights 🔬

- **Lamport Clock Synchronization** for ordering and mutual exclusion.
- **Distributed Queue** to manage critical-section access for updates.
- **Efficient Broadcast** of updates to maintain replica synchronization.
- **Fault-Tolerant Design** ready for simple peer-failure recovery (future work).

---

## Scenario Example 🎉

1. **Sensor** generates a new reading (e.g., 25°C on April 25th).
2. It requests critical section access using its Lamport timestamp.
3. Once access is granted, it inserts the new data and multicasts the update.
4. **Verifiers** independently scan local replicas, detect anomalies (e.g., an impossible 200°C reading), and correct them through the same distributed locking mechanism.
5. **Users** read all local data to predict the temperature for April 26th without needing to lock.

---

## Data Flow 🌊 (abstract level)

Below is a flowchart representing the broadcasting of a sensor data, then verification of this data with request & release.
From reading the chart, it can be seen that *only the verifier nodes and sensors* need to send data, thus need to request & release data.
Users only receive and update their local data replica.

```mermaid
sequenceDiagram
    participant S as SensorNode
    participant V1 as VerifierNode 1
    participant V2 as VerifierNode 2
    participant U as UserNode
    
    S->>S: Generate temperature reading
    S->>+V1: Broadcast reading
    S->>+V2: Broadcast reading
    S->>+U: Broadcast reading
    
    Note over U: Store reading in local datastore
    Note over V1,V2: Store reading in local datastores
    
    V1->>V2: Request lock for day X (Lamport algorithm)
    V2->>V1: Acknowledge lock
    
    Note over V1: Verify readings for day X
    
    V1->>V1: Correct invalid readings
    V1->>V2: Release lock & broadcast verified data
    V1->>U: Broadcast verified data
    
    Note over V2,U: Update local data stores
    
    V2->>V1: Request lock for day Y
    V1->>V2: Acknowledge lock
    
    Note over V2: Verify readings for day Y
    
    V2->>V2: Correct invalid readings
    V2->>V1: Release lock & broadcast verified data
    V2->>U: Broadcast verified data
    
    Note over V1,U: Update local data stores
    
    U->>U: Process all available data (verified & unverified)
    Note over U: Generate weather prediction
```

## Class Diagram 🔬

Below is a proposition of class diagram.

- Senrors, Verifiers, Users are all Nodes, thus share a basic structure (Node class), 
  and has their own DataStore.
- Sensors produce Readings
- Users produce WeatherPrediction
- Nodes shares Message
- clocks are represented via LamportClock (as the Lamport algorithm might be used)

*Protected elements of Nodes (ie with #) are elements that might be used to help process and send the messages.
They might be changed to a dedicated control layer.*

```mermaid
classDiagram
    class Node {
        <<interface>>
        +Start() error
        +ID() string
        +Type() string
        +HandleMessage(chan string)
        +GetName() string
        +SetControlLayer(ControlLayer) error
    }

    class BaseNode {
        -id string
        -nodeType string
        -isRunning bool
        -clock int
        -ctrlLayer ControlLayer
        -nbMsgSent int
        +ID() string
        +Type() string
        +GetName() string
        +SetControlLayer(ControlLayer) error
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
        -processingCapacity int
        -threshold float64
        -verificationLocks map
        -lockRequests map
        -lockReplies map
        -otherVerifiers map
        +Start() error
        +HandleMessage(chan string)
    }

    class UserNode {
        -predictionModel string
        -predictionWindow time.Duration
        -predictionInterval time.Duration
        -lastPrediction *float32
        +Start() error
        +HandleMessage(chan string)
    }

    class ControlLayer {
        -id string
        -nodeType string
        -isRunning bool
        -clock int
        -child Node
        -nbMsgSent int
        -seenIDs map
        -channel_to_application chan string
        +GetName() string
        +Start() error
        +HandleMessage(string) error
        +AddNewMessageId(string)
        +SendMsgFromApplication(string) error
    }
	
    class Reading {
        +Temperature float64
        +Timestamp time.Time
        +SensorID string
        +IsVerified bool
    }

    %% Relationships
    Node <|.. BaseNode : implements
    BaseNode <|-- SensorNode : extends
    BaseNode <|-- VerifierNode : extends
    BaseNode <|-- UserNode : extends
    
    ControlLayer "1" *-- "1" Node : contains >
    BaseNode "1" o-- "1" ControlLayer : references >
    SensorNode ..> Reading : creates >

    %% Control relationships
    ControlLayer ..> "SendMsgFromApplication" Node : forwards messages to >
    Node ..> "HandleMessage" ControlLayer : sends messages via >
```


## Synchronisation algorithm (Lamport) 🔬

Below if a sequence diagram of the synchronisation algorithme where Verifiers request, check, update and release data.
They only have to request data exclusivity to other Verifiers (ie not Sensors nor Users), as only Verifiers updates data.


```mermaid
sequenceDiagram
    participant V1 as Verifier 1
    participant V2 as Verifier 2
    participant V3 as Verifier 3
    
    Note over V1,V3: Each verifier has its own Lamport clock
    
    V1->>+V1: Wants to verify data for Day 1
    V1->>V2: REQUEST(V1, Day 1, timestamp=10)
    V1->>V3: REQUEST(V1, Day 1, timestamp=10)
    
    V2->>V2: Update clock to max(local, received)+1 = 11
    V3->>V3: Update clock to max(local, received)+1 = 11
    
    V2->>V1: REPLY(V2, Day 1, timestamp=11)
    V3->>V1: REPLY(V3, Day 1, timestamp=11)
    
    Note over V1: Received all replies, can proceed with verification
    
    V1->>V1: Verify and correct data for Day 1
    
    V1->>V2: RELEASE(V1, Day 1, timestamp=12)
    V1->>V3: RELEASE(V1, Day 1, timestamp=12)
    V1->>V2: BROADCAST_VERIFIED_DATA(Day 1, corrected readings)
    V1->>V3: BROADCAST_VERIFIED_DATA(Day 1, corrected readings)
    
    V2->>V2: Update local data store
    V3->>V3: Update local data store
    
    Note over V2: Now wants to verify Day 2
    
    V2->>V1: REQUEST(V2, Day 2, timestamp=13)
    V2->>V3: REQUEST(V2, Day 2, timestamp=13)
    
    V1->>V2: REPLY(V1, Day 2, timestamp=14)
    V3->>V2: REPLY(V3, Day 2, timestamp=14)
    
    Note over V2: Received all replies, can proceed with verification
```
