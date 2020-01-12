package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	fleet "github.com/synerex/proto_fleet"
	pt "github.com/synerex/proto_ptransit"
	api "github.com/synerex/synerex_api"
	pbase "github.com/synerex/synerex_proto"
	sxutil "github.com/synerex/synerex_sxutil"
)

// PT2Fleet provider converts Public Transit information into Fleet protocol.

var (
	nodesrv         = flag.String("nodesrv", "127.0.0.1:9990", "Node ID Server")
	mu              sync.Mutex
	version         = "0.01"
	pktCount        = 0
	rideClient      *sxutil.SXServiceClient
	sxServerAddress string
)

func supplyPTCallback(client *sxutil.SXServiceClient, sp *api.Supply) {
	pt := &pt.PTService{}
	err := proto.Unmarshal(sp.Cdata.Entity, pt)

	if err == nil { // get PT
		//		fmt.Printf("Receive PT: %#v", *pt)

		fleet := fleet.Fleet{
			VehicleId: pt.VehicleId,
			Angle:     float32(pt.Angle),
			Speed:     int32(pt.Speed),
			Status:    int32(0),
			Coord: &fleet.Fleet_Coord{
				Lat: float32(pt.Lat),
				Lon: float32(pt.Lon),
			},
		}
		//		fmt.Printf("Fleet: %#v", fleet)

		out, err2 := proto.Marshal(&fleet)
		if err2 == nil {
			cont := api.Content{Entity: out}
			// Register supply
			smo := sxutil.SupplyOpts{
				Name:  "Fleet Supply",
				Cdata: &cont,
			}
			//			fmt.Printf("Res: %v",smo)
			_, nerr := rideClient.NotifySupply(&smo) // send to fleet
			if nerr != nil {                         // connection failuer with current client
				// we need to ask to nodeidserv?
				// or just reconnect.
				time.Sleep(5 * time.Second)
				newClient := sxutil.GrpcConnectServer(sxServerAddress)
				if newClient != nil {
					log.Printf("Reconnect Server %s\n", sxServerAddress)
					rideClient.Client = newClient
				}
			} else { // sent OK!
				pktCount++
			}
		} else {
			log.Printf("PB Fleet Marshal Error! %v", err)
		}
	}
}

func subscribePTSupply(client *sxutil.SXServiceClient) {
	for { // for reconnect adaption
		ctx := context.Background() //
		err := client.SubscribeSupply(ctx, supplyPTCallback)
		log.Printf("Error:Supply %s\n", err.Error())
		// we need to restart

		time.Sleep(5 * time.Second) // wait 5 seconds to reconnect
		newClt := sxutil.GrpcConnectServer(sxServerAddress)
		if newClt != nil {
			log.Printf("Reconnect server [%s]\n", sxServerAddress)
			client.Client = newClt
		}
	}
}

// just for stat debug
func monitorStatus() {
	for {
		sxutil.SetNodeStatus(int32(pktCount), "<-count")
		time.Sleep(time.Second * 3)
	}
}

func main() {
	flag.Parse()
	go sxutil.HandleSigInt()
	sxutil.RegisterDeferFunction(sxutil.UnRegisterNode)

	channelTypes := []uint32{pbase.RIDE_SHARE}
	srv, rerr := sxutil.RegisterNode(*nodesrv, "PT2Fleet", channelTypes, nil)
	if rerr != nil {
		log.Fatal("Can't register node ", rerr)
	}
	log.Printf("Connecting SynerexServer at [%s]\n", srv)
	wg := sync.WaitGroup{} // for syncing other goroutines

	client := sxutil.GrpcConnectServer(srv)
	sxServerAddress = srv

	argJSON2 := fmt.Sprintf("{PT2Fleet:PTransit}")
	ptClient := sxutil.NewSXServiceClient(client, pbase.PT_SERVICE, argJSON2)

	log.Printf("Starting PT2Fleet Provider %s", version)
	wg.Add(1)
	go subscribePTSupply(ptClient)

	argJSON := fmt.Sprintf("{PT2Fleet:Fleet}")
	rideClient = sxutil.NewSXServiceClient(client, pbase.RIDE_SHARE, argJSON)

	go monitorStatus() // keep status

	wg.Wait()

}
