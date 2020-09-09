// Binary raftadmin is a CLI interface to the RaftAdmin gRPC service.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	pb "github.com/Jille/raftadmin/proto"
	"github.com/iancoleman/strcase"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/reflect/protoreflect"

	// Allow dialing multiple nodes with multi:///.
	_ "github.com/Jille/grpc-multi-resolver"

	// Register health checker with gRPC.
	_ "google.golang.org/grpc/health"
)

func main() {
	if err := do(); err != nil {
		log.Fatal(err)
	}
}

// There is no way to go from a protoreflect.MessageDescriptor to an instance of the message :(
var protoTypes = []protoreflect.ProtoMessage{
	&pb.Future{},
	&pb.AwaitResponse{},
	&pb.ForgetResponse{},
	&pb.AddVoterRequest{},
	&pb.AddNonvoterRequest{},
	&pb.ApplyLogRequest{},
	&pb.AppliedIndexRequest{},
	&pb.AppliedIndexResponse{},
	&pb.BarrierRequest{},
	&pb.DemoteVoterRequest{},
	&pb.GetConfigurationRequest{},
	&pb.GetConfigurationResponse{},
	&pb.LastContactRequest{},
	&pb.LastContactResponse{},
	&pb.LastIndexRequest{},
	&pb.LastIndexResponse{},
	&pb.LeaderRequest{},
	&pb.LeaderResponse{},
	&pb.LeadershipTransferRequest{},
	&pb.LeadershipTransferToServerRequest{},
	&pb.RemoveServerRequest{},
	&pb.ShutdownRequest{},
	&pb.SnapshotRequest{},
	&pb.StateRequest{},
	&pb.StateResponse{},
	&pb.StatsRequest{},
	&pb.StatsResponse{},
	&pb.VerifyLeaderRequest{},
}

// messageFromDescriptor creates a new Message for a MessageDescriptor.
func messageFromDescriptor(d protoreflect.MessageDescriptor) protoreflect.Message {
	for _, m := range protoTypes {
		if m.ProtoReflect().Descriptor() == d {
			return m.ProtoReflect().New()
		}
	}
	panic(fmt.Errorf("unknown type %q; please add it to protoTypes", d.FullName()))
}

func do() error {
	ctx := context.Background()
	methods := pb.File_raftadmin_proto.Services().ByName("RaftAdmin").Methods()
	leader := flag.Bool("leader", false, "Whether to dial to the leader (requires https://github.com/Jille/raft-grpc-leader-rpc)")
	healthCheckService := flag.String("health_check_service", "quis.RaftLeader", "Which gRPC service to health check when searching for the leader")
	flag.Parse()

	if flag.NArg() < 2 {
		var commands []string
		for i := 0; methods.Len() > i; i++ {
			commands = append(commands, strcase.ToSnake(string(methods.Get(i).Name())))
		}
		sort.Strings(commands)
		return fmt.Errorf("Usage: raftadmin <host:port> <command> <args...>\nCommands: %s", strings.Join(commands, ", "))
	}

	target := flag.Arg(0)
	command := flag.Arg(1)
	// Look up the command as CamelCase and as-is (usually snake_case).
	m := methods.ByName(protoreflect.Name(command))
	if m == nil {
		m = methods.ByName(protoreflect.Name(strcase.ToCamel(command)))
	}
	if m == nil {
		return fmt.Errorf("unknown command %q", command)
	}

	// Sort fields by field number.
	reqDesc := m.Input()
	unorderedFields := reqDesc.Fields()
	fields := make([]protoreflect.FieldDescriptor, unorderedFields.Len())
	for i := 0; unorderedFields.Len() > i; i++ {
		f := unorderedFields.Get(i)
		fields[f.Number()-1] = f
	}
	if flag.NArg() != 2+len(fields) {
		var names []string
		for _, f := range fields {
			names = append(names, fmt.Sprintf("<%s>", f.TextName()))
		}
		return fmt.Errorf("Usage: raftadmin <host:port> %s %s", command, strings.Join(names, " "))
	}

	// Convert given strings to the right type and set them on the request proto.
	req := messageFromDescriptor(reqDesc)
	for i, f := range fields {
		s := flag.Arg(2 + i)
		var v protoreflect.Value
		switch f.Kind() {
		case protoreflect.StringKind:
			v = protoreflect.ValueOfString(s)
		case protoreflect.BytesKind:
			v = protoreflect.ValueOfBytes([]byte(s))
		case protoreflect.Uint64Kind:
			i, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				return err
			}
			v = protoreflect.ValueOfUint64(uint64(i))
		default:
			return fmt.Errorf("internal error: kind %s is not yet supported", f.Kind().String())
		}
		req.Set(f, v)
	}

	// Connect and send the RPC.
	var o grpc.DialOption = grpc.EmptyDialOption{}
	if *leader {
		o = grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"healthCheckConfig": {"serviceName": "%s"}, "loadBalancingConfig": [ { "round_robin": {} } ]}`, *healthCheckService))
	}
	conn, err := grpc.Dial(target, grpc.WithInsecure(), grpc.WithBlock(), o)
	if err != nil {
		return err
	}
	defer conn.Close()

	log.Printf("Invoking %s(%s)", m.Name(), prototext.Format(req.Interface()))
	resp := messageFromDescriptor(m.Output()).Interface()
	if err := conn.Invoke(ctx, "/RaftAdmin/"+string(m.Name()), req.Interface(), resp); err != nil {
		return err
	}
	log.Printf("Response: %s", prototext.Format(resp))

	// This method returned a future. We should call Await to get the result, and then Forget to free up the memory of the server.
	if f, ok := resp.(*pb.Future); ok {
		c := pb.NewRaftAdminClient(conn)
		log.Printf("Invoking Await(%s)", prototext.Format(f))
		resp, err := c.Await(ctx, f)
		if err != nil {
			return err
		}
		log.Printf("Response: %s", prototext.Format(resp))
		if _, err := c.Forget(ctx, f); err != nil {
			return err
		}
	}
	return nil
}
