package full_sync

import (
	"time"
	"fmt"

	"nimo-shake/protocal"
	"nimo-shake/common"
	"nimo-shake/configure"
	"nimo-shake/writer"

	LOG "github.com/vinllen/log4go"
)

const (
	batchNumber  = 512
	batchSize    = 2 * utils.MB // mongodb limit: 16MB
	batchTimeout = 1            // seconds
)

/*------------------------------------------------------*/
// one document link corresponding to one documentSyncer
type documentSyncer struct {
	tableSyncerId int
	id            int // documentSyncer id
	ns            utils.NS
	inputChan     chan interface{} // parserChan in table-syncer
	writer        writer.Writer
}

func NewDocumentSyncer(tableSyncerId int, table string, id int, inputChan chan interface{}) *documentSyncer {
	ns := utils.NS{
		Database:   conf.Options.Id,
		Collection: table,
	}

	writer := writer.NewWriter(conf.Options.TargetType, conf.Options.TargetAddress, ns, conf.Options.LogLevel)
	if writer == nil {
		LOG.Crashf("tableSyncer[%v] documentSyncer[%v] create writer failed", tableSyncerId, table)
	}

	return &documentSyncer{
		tableSyncerId: tableSyncerId,
		id:            id,
		inputChan:     inputChan,
		writer:        writer,
		ns:            ns,
	}
}

func (ds *documentSyncer) String() string {
	return fmt.Sprintf("tableSyncer[%v] documentSyncer[%v] ns[%v]", ds.tableSyncerId, ds.id, ds.ns)
}

func (ds *documentSyncer) Close() {
	ds.writer.Close()
}

func (ds *documentSyncer) Run() {
	var data interface{}
	var ok bool
	batchGroup := make([]interface{}, 0, batchNumber)
	timeout := false
	batchGroupSize := 0
	exit := false
	for {
		select {
		case data, ok = <-ds.inputChan:
			if !ok {
				exit = true
				LOG.Info("%s channel already closed, flushing cache and exiting...", ds.String())
			}
		case <-time.After(time.Second * batchTimeout):
			// timeout
			timeout = true
		}

		LOG.Debug("exit[%v], timeout[%v], len(batchGroup)[%v], batchGroupSize[%v], data[%v]", exit, timeout,
			len(batchGroup), batchGroupSize, data)

		switch v := data.(type) {
		case protocal.RawData:
			if v.Size > 0 {
				batchGroup = append(batchGroup, v.Data)
				batchGroupSize += v.Size
			}
		}

		if exit || timeout || len(batchGroup) >= batchNumber || batchGroupSize >= batchSize {
			if len(batchGroup) != 0 {
				if err := ds.write(batchGroup); err != nil {
					LOG.Crashf("%s write data failed[%v]", ds.String(), err)
				}

				batchGroup = make([]interface{}, 0, batchNumber)
				batchGroupSize = 0
			}

			if exit {
				break
			}
			timeout = false
		}
	}

	LOG.Info("%s finish writing", ds.String())
}

func (ds *documentSyncer) write(input []interface{}) error {
	LOG.Debug("%s writing data with length[%v]", ds.String(), len(input))
	if len(input) == 0 {
		return nil
	}

	return ds.writer.WriteBulk(input)
}
