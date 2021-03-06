package operator

import (
	"github.com/spotahome/kooper/client/crd"
	"github.com/spotahome/kooper/monitoring/metrics"
	"github.com/spotahome/kooper/operator"
	"github.com/spotahome/kooper/operator/controller"
	"github.com/spotahome/kooper/operator/resource"
	"github.com/spotahome/kooper/operator/retrieve"
	baas8scli "github.com/vshn/k8up/client/k8s/clientset/versioned"
	"github.com/vshn/k8up/log"
	"github.com/vshn/k8up/monitoring"
	"github.com/vshn/k8up/service"
	"github.com/vshn/k8up/service/archive"
	"github.com/vshn/k8up/service/backup"
	"github.com/vshn/k8up/service/check"
	"github.com/vshn/k8up/service/observe"
	"github.com/vshn/k8up/service/prune"
	"github.com/vshn/k8up/service/restore"
	"github.com/vshn/k8up/service/schedule"
	"k8s.io/client-go/kubernetes"
)

type options struct {
	cfg Config
	clients
	logger  log.Logger
	metrics *metrics.Prometheus
}

type clients struct {
	baasCLI baas8scli.Interface
	crdCli  crd.Interface
	kubeCli kubernetes.Interface
}

// New returns a new K8up operator
func New(cfg Config, baasCLI baas8scli.Interface, crdCli crd.Interface, kubeCli kubernetes.Interface, logger log.Logger) (operator.Operator, error) {

	options := options{
		cfg: cfg,
		clients: clients{
			baasCLI: baasCLI,
			crdCli:  crdCli,
			kubeCli: kubeCli,
		},
		logger:  logger,
		metrics: monitoring.GetInstance().Metrics,
	}

	operators := create(options)

	return operators, nil
}

func create(options options) operator.Operator {

	commonObjects := service.CommonObjects{
		BaasCLI: options.baasCLI,
		K8sCli:  options.kubeCli,
		Logger:  options.logger,
	}

	bCRD := newBackupCRD(options.clients)
	backup := backup.NewBackup(commonObjects, observe.GetInstance())
	backupHandler := newHandler(options.logger, backup)

	rCRD := newRestoreCRD(options.clients)
	restore := restore.NewRestore(commonObjects, observe.GetInstance())
	restoreHandler := newHandler(options.logger, restore)

	aCRD := newArchiveCRD(options.clients)
	archive := archive.NewArchive(commonObjects, observe.GetInstance())
	archiveHandler := newHandler(options.logger, archive)

	sCRD := newScheduleCRD(options.clients)
	schedule := schedule.NewSchedule(commonObjects, observe.GetInstance())
	scheduleHandler := newHandler(options.logger, schedule)

	pod := newPodObserve(options.clients)
	podObserver := observe.GetInstance()
	podObserver.SetCommonObjects(commonObjects)
	podObserverHandler := newHandler(options.logger, podObserver)

	job := newJobObserve(options.clients)
	jobObserver := observe.GetInstance()
	jobObserverHandler := newHandler(options.logger, jobObserver)

	cCRD := newCheckCRD(options.clients)
	check := check.NewCheck(commonObjects, observe.GetInstance())
	checkHandler := newHandler(options.logger, check)

	pCRD := newPruneCRD(options.clients)
	prune := prune.NewPruner(commonObjects, observe.GetInstance())
	pruneHandler := newHandler(options.logger, prune)

	ptCRD := newPreBackupPodCRD(options.clients)

	CRDs := []resource.CRD{
		bCRD,
		rCRD,
		aCRD,
		sCRD,
		pod,
		job,
		cCRD,
		pCRD,
	}

	cfg := []controller.Config{
		{
			ProcessingJobRetries: 5,
			ResyncInterval:       options.cfg.ResyncPeriod,
			ConcurrentWorkers:    1,
			Name:                 "backup",
		},
		{
			ProcessingJobRetries: 5,
			ResyncInterval:       options.cfg.ResyncPeriod,
			ConcurrentWorkers:    1,
			Name:                 "restore",
		},
		{
			ProcessingJobRetries: 5,
			ResyncInterval:       options.cfg.ResyncPeriod,
			ConcurrentWorkers:    1,
			Name:                 "archive",
		},
		{
			ProcessingJobRetries: 5,
			ResyncInterval:       options.cfg.ResyncPeriod,
			ConcurrentWorkers:    1,
			Name:                 "schedule",
		},
		{
			ProcessingJobRetries: 5,
			ResyncInterval:       options.cfg.ResyncPeriod,
			ConcurrentWorkers:    1,
			Name:                 "podObserver",
		},
		{
			ProcessingJobRetries: 5,
			ResyncInterval:       options.cfg.ResyncPeriod,
			ConcurrentWorkers:    1,
			Name:                 "jobObserver",
		},
		{
			ProcessingJobRetries: 5,
			ResyncInterval:       options.cfg.ResyncPeriod,
			ConcurrentWorkers:    1,
			Name:                 "check",
		},
		{
			ProcessingJobRetries: 5,
			ResyncInterval:       options.cfg.ResyncPeriod,
			ConcurrentWorkers:    1,
			Name:                 "prune",
		},
	}

	retr := []retrieve.Resource{
		{
			Object:        bCRD.GetObject(),
			ListerWatcher: bCRD.GetListerWatcher(),
		},
		{
			Object:        rCRD.GetObject(),
			ListerWatcher: rCRD.GetListerWatcher(),
		},
		{
			Object:        aCRD.GetObject(),
			ListerWatcher: aCRD.GetListerWatcher(),
		},
		{
			Object:        sCRD.GetObject(),
			ListerWatcher: sCRD.GetListerWatcher(),
		},
		{
			Object:        pod.GetObject(),
			ListerWatcher: pod.GetListerWatcher(),
		},
		{
			Object:        job.GetObject(),
			ListerWatcher: job.GetListerWatcher(),
		},
		{
			Object:        cCRD.GetObject(),
			ListerWatcher: cCRD.GetListerWatcher(),
		},
		{
			Object:        pCRD.GetObject(),
			ListerWatcher: pCRD.GetListerWatcher(),
		},
	}

	// Register non-controlled CRDs
	err := ptCRD.Initialize()
	if err != nil {
		options.logger.Errorf("Error initializing podTemplate CRD:", err)
	}

	ctrls := []controller.Controller{}
	ctrls = append(ctrls, controller.New(&cfg[0], backupHandler, &retr[0], nil, nil, options.metrics, options.logger))
	ctrls = append(ctrls, controller.New(&cfg[1], restoreHandler, &retr[1], nil, nil, options.metrics, options.logger))
	ctrls = append(ctrls, controller.New(&cfg[2], archiveHandler, &retr[2], nil, nil, options.metrics, options.logger))
	ctrls = append(ctrls, controller.New(&cfg[3], scheduleHandler, &retr[3], nil, nil, options.metrics, options.logger))
	ctrls = append(ctrls, controller.New(&cfg[4], podObserverHandler, &retr[4], nil, nil, options.metrics, options.logger))
	ctrls = append(ctrls, controller.New(&cfg[5], jobObserverHandler, &retr[5], nil, nil, options.metrics, options.logger))
	ctrls = append(ctrls, controller.New(&cfg[6], checkHandler, &retr[6], nil, nil, options.metrics, options.logger))
	ctrls = append(ctrls, controller.New(&cfg[7], pruneHandler, &retr[7], nil, nil, options.metrics, options.logger))
	return operator.NewMultiOperator(CRDs, ctrls, options.logger)
}
