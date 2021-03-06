package disk

import (
	"fmt"
	"github.com/nilebit/bitstore/disk/volume"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/golang/glog"
)

type Location struct {
	Directory      	string
	MaxVolumeCount 	int
	volumes        	map[volume.VIDType]*volume.Volume
	sync.RWMutex
}

func NewLocation(dir string, maxVolumeCount int) *Location {
	location := &Location{Directory: dir, MaxVolumeCount: maxVolumeCount}
	location.volumes = make(map[volume.VIDType]*volume.Volume)
	return location
}

func (l *Location) volumeIdFromPath(dir os.FileInfo) (volume.VIDType, string, error) {
	name := dir.Name()
	if dir.IsDir() || !strings.HasSuffix(name, ".dat") {
		return 0, "", fmt.Errorf("Path is not a volume: %s", name)
	}

	collection := ""
	base := name[:len(name)-len(".dat")]
	i := strings.LastIndex(base, "_")
	if i > 0 {
		collection, base = base[0:i], base[i+1:]
	}
	vol, err := volume.NewVolumeId(base)

	return vol, collection, err
}

func (l *Location) loadExistingVolume(dir os.FileInfo, mutex *sync.RWMutex) {
	name := dir.Name()
	if dir.IsDir() || !strings.HasSuffix(name, ".dat") {
		return
	}

	vid, collection, err := l.volumeIdFromPath(dir)
	if err != nil {
		return
	}

	mutex.RLock()
	_, found := l.volumes[vid]
	mutex.RUnlock()
	if !found {
		if v, e := volume.NewVolume(l.Directory, collection, vid, nil, nil, 0); e == nil {
			mutex.Lock()
			l.volumes[vid] = v
			mutex.Unlock()
			glog.V(0).Infof("data file %s, replicaPlacement=%s v=%d size=%d ttl=%s",
				l.Directory+"/"+name, v.ReplicaPlacement, v.Version(), v.Size(), v.Ttl.String())
		} else {
			glog.V(0).Infof("new volume %s error %s", name, e)
		}
	}
}

func (l *Location) concurrentLoadingVolumes(concurrentFlag bool) {
	var concurrency int
	if concurrentFlag {
		//You could choose a better optimized concurency value after testing at your environment
		concurrency = 10
	} else {
		concurrency = 1
	}

	task_queue := make(chan os.FileInfo, 10*concurrency)
	go func() {
		if dirs, err := ioutil.ReadDir(l.Directory); err == nil {
			for _, dir := range dirs {
				task_queue <- dir
			}
		}
		close(task_queue)
	}()

	var wg sync.WaitGroup
	var mutex sync.RWMutex
	for workerNum := 0; workerNum < concurrency; workerNum++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for dir := range task_queue {
				l.loadExistingVolume(dir, &mutex)
			}
		}()
	}
	wg.Wait()

}

func (l *Location) loadExistingVolumes() {
	l.Lock()
	defer l.Unlock()

	l.concurrentLoadingVolumes(true)

	glog.V(0).Infoln("Store started on dir:", l.Directory, "with", len(l.volumes), "volumes", "max", l.MaxVolumeCount)
}



