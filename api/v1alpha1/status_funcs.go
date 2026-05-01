package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *Cluster) StatusValue() any                      { return c.Status }
func (c *Cluster) StatusConditions() *[]metav1.Condition { return &c.Status.Conditions }
func (c *Cluster) SetObservedGeneration(g int64)         { c.Status.ObservedGeneration = g }
func (c *Cluster) AssignStatusFrom(o client.Object) {
	if other, ok := o.(*Cluster); ok {
		c.Status = other.Status
	}
}

func (p *Project) StatusValue() any                      { return p.Status }
func (p *Project) StatusConditions() *[]metav1.Condition { return &p.Status.Conditions }
func (p *Project) SetObservedGeneration(g int64)         { p.Status.ObservedGeneration = g }
func (p *Project) AssignStatusFrom(o client.Object) {
	if other, ok := o.(*Project); ok {
		p.Status = other.Status
	}
}

func (b *Branch) StatusValue() any                      { return b.Status }
func (b *Branch) StatusConditions() *[]metav1.Condition { return &b.Status.Conditions }
func (b *Branch) SetObservedGeneration(g int64)         { b.Status.ObservedGeneration = g }
func (b *Branch) AssignStatusFrom(o client.Object) {
	if other, ok := o.(*Branch); ok {
		b.Status = other.Status
	}
}

func (p *Pageserver) StatusValue() any                      { return p.Status }
func (p *Pageserver) StatusConditions() *[]metav1.Condition { return &p.Status.Conditions }
func (p *Pageserver) SetObservedGeneration(g int64)         { p.Status.ObservedGeneration = g }
func (p *Pageserver) AssignStatusFrom(o client.Object) {
	if other, ok := o.(*Pageserver); ok {
		p.Status = other.Status
	}
}

func (s *Safekeeper) StatusValue() any                      { return s.Status }
func (s *Safekeeper) StatusConditions() *[]metav1.Condition { return &s.Status.Conditions }
func (s *Safekeeper) SetObservedGeneration(g int64)         { s.Status.ObservedGeneration = g }
func (s *Safekeeper) AssignStatusFrom(o client.Object) {
	if other, ok := o.(*Safekeeper); ok {
		s.Status = other.Status
	}
}
