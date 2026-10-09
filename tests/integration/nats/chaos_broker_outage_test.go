//go:build integration

package nats_test

import (
 "context"
 "fmt"
 "time"

 natsgo "github.com/nats-io/nats.go"
 "github.com/nats-io/nats.go/jetstream"
 "github.com/testcontainers/testcontainers-go"
 "github.com/testcontainers/testcontainers-go/wait"
 . "github.com/onsi/ginkgo/v2"
 . "github.com/onsi/gomega"
)

var _ = Describe("CHAOS :: NATS JETSTREAM OUTAGE",func(){
 It("should reject publish during broker outage and resume after restart",func(){
  ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second)
  defer cancel()
  broker,err:=testcontainers.GenericContainer(ctx,testcontainers.GenericContainerRequest{
   Started:true,
   ContainerRequest:testcontainers.ContainerRequest{
    Image:"nats:2.11-alpine",
    ExposedPorts:[]string{"4222/tcp"},
    Cmd:[]string{"-js","-sd","/tmp/jetstream"},
    WaitingFor:wait.ForListeningPort("4222/tcp"),
   },
  })
  Expect(err).NotTo(HaveOccurred())
  DeferCleanup(func(){
   cleanupCtx,cleanupCancel:=context.WithTimeout(context.Background(),20*time.Second)
   defer cleanupCancel()
   Expect(broker.Terminate(cleanupCtx)).To(Succeed())
  })
  host,err:=broker.Host(ctx)
  Expect(err).NotTo(HaveOccurred())
  port,err:=broker.MappedPort(ctx,"4222/tcp")
  Expect(err).NotTo(HaveOccurred())
  url:=fmt.Sprintf("nats://%s:%s",host,port.Port())

  nc,err:=natsgo.Connect(url,natsgo.Timeout(3*time.Second),natsgo.NoReconnect())
  Expect(err).NotTo(HaveOccurred())
  js,err:=jetstream.New(nc)
  Expect(err).NotTo(HaveOccurred())
  _,err=js.CreateStream(ctx,jetstream.StreamConfig{
   Name:"CHAOS_LEDGER",
   Subjects:[]string{"ledger.transaction.created"},
  })
  Expect(err).NotTo(HaveOccurred())
  first,err:=js.Publish(ctx,"ledger.transaction.created",[]byte("before-outage"),jetstream.WithMsgID("chaos-event-1"))
  Expect(err).NotTo(HaveOccurred())
  Expect(first.Sequence).To(Equal(uint64(1)))

  Expect(broker.Stop(ctx,nil)).To(Succeed())
  outageCtx,outageCancel:=context.WithTimeout(ctx,1500*time.Millisecond)
  defer outageCancel()
  _,err=js.Publish(outageCtx,"ledger.transaction.created",[]byte("during-outage"),jetstream.WithMsgID("chaos-event-2"))
  Expect(err).To(HaveOccurred(),"JetStream must not acknowledge publication while unavailable")
  nc.Close()

  Expect(broker.Start(ctx)).To(Succeed())

  // Docker may assign a different published host port when a container is
  // started again. Never reuse the mapped address captured before Stop.
  // Refresh it on every probe so delayed port publication is also covered.
  recoveredURL:=""
  Eventually(func(g Gomega){
   currentHost,hostErr:=broker.Host(ctx)
   g.Expect(hostErr).NotTo(HaveOccurred())
   currentPort,portErr:=broker.MappedPort(ctx,"4222/tcp")
   g.Expect(portErr).NotTo(HaveOccurred())
   endpoint:=fmt.Sprintf("nats://%s:%s",currentHost,currentPort.Port())
   probe,connErr:=natsgo.Connect(endpoint,natsgo.Timeout(time.Second),natsgo.NoReconnect())
   g.Expect(connErr).NotTo(HaveOccurred())
   probe.Close()
   recoveredURL=endpoint
  },20*time.Second,300*time.Millisecond).Should(Succeed())

  recovered,err:=natsgo.Connect(recoveredURL,natsgo.Timeout(3*time.Second),natsgo.NoReconnect())
  Expect(err).NotTo(HaveOccurred())
  defer recovered.Close()
  jsRecovered,err:=jetstream.New(recovered)
  Expect(err).NotTo(HaveOccurred())
  second,err:=jsRecovered.Publish(ctx,"ledger.transaction.created",[]byte("after-restart"),jetstream.WithMsgID("chaos-event-3"))
  Expect(err).NotTo(HaveOccurred())
  Expect(second.Sequence).To(Equal(uint64(2)),"only confirmed events should be present")
  recoveredStream,err:=jsRecovered.Stream(ctx,"CHAOS_LEDGER")
  Expect(err).NotTo(HaveOccurred())
  info,err:=recoveredStream.Info(ctx)
  Expect(err).NotTo(HaveOccurred())
  Expect(info.State.Msgs).To(Equal(uint64(2)))
 })
})
