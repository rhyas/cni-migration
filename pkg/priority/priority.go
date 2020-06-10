package priority

import (
	"context"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/joshvanl/cni-migration/pkg"
	"github.com/joshvanl/cni-migration/pkg/config"
	"github.com/joshvanl/cni-migration/pkg/util"
)

var _ pkg.Step = &Priority{}

type Priority struct {
	ctx context.Context
	log *logrus.Entry

	config  *config.Config
	client  *kubernetes.Clientset
	factory *util.Factory
}

func New(ctx context.Context, config *config.Config) pkg.Step {
	log := config.Log.WithField("step", "3-priority")
	return &Priority{
		log:     log,
		ctx:     ctx,
		config:  config,
		client:  config.Client,
		factory: util.New(ctx, log, config.Client),
	}
}

// Ready ensures that
// - All nodes have the revered cni-priority-cilium label
func (p *Priority) Ready() (bool, error) {
	nodes, err := p.client.CoreV1().Nodes().List(p.ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}

	for _, n := range nodes.Items {
		if !p.hasRequiredLabel(n.Labels) {
			return false, nil
		}
	}

	p.log.Info("step 3 ready")

	return true, nil
}

func (p *Priority) Run(dryrun bool) error {
	p.log.Info("reversing priority of CNI to cilium...")

	if !dryrun {
		if err := p.factory.CheckKnetStress(); err != nil {
			return err
		}
	}

	nodes, err := p.client.CoreV1().Nodes().List(p.ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, n := range nodes.Items {
		if !p.hasRequiredLabel(n.Labels) {
			p.log.Infof("changing CNI priority to Cilium on node %s", n.Name)
			if err := p.node(dryrun, n.Name); err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *Priority) node(dryrun bool, name string) error {
	node, err := p.client.CoreV1().Nodes().Get(p.ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	p.log.Infof("adding Cilium primary CNI label to node %s", name)
	if !dryrun {
		if node.Labels == nil {
			node.Labels = make(map[string]string)
		}
		delete(node.Labels, p.config.Labels.CNIPriorityCanal)
		node.Labels[p.config.Labels.CNIPriorityCilium] = p.config.Labels.Value

		_, err = p.client.CoreV1().Nodes().Update(p.ctx, node, metav1.UpdateOptions{})
		if err != nil {
			return err
		}

	}

	if err := p.factory.RollNode(dryrun, name, p.config.WatchedResources); err != nil {
		return err
	}

	return nil
}

func (p *Priority) hasRequiredLabel(labels map[string]string) bool {
	if labels == nil {
		return false
	}

	if v, ok := labels[p.config.Labels.CNIPriorityCilium]; !ok || v != p.config.Labels.Value {
		return false
	}

	return true
}