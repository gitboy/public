// hosts: potentiallinstnacey useful tool for Sessioning AWS infrastructure.
// concurrent queries across all AWS regions -> fast!
// you'll need sufficiently privileged AWS credentials
// author: russd
// vim:ts=4

package main

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/docopt/docopt-go"
	"log"
	"strings"
	"sync"
	"time"
)

const VERSION string = "hosts-1.0"

type Session struct {
	s *session.Session
}

func NewSession() *Session {
	sess, err := session.NewSession()
	die(err)
	return &Session{s: sess}
}

func collect(output *ec2.DescribeInstancesOutput, results chan *ec2.Instance) {
	for i, _ := range output.Reservations {
		for _, inst := range output.Reservations[i].Instances {
			results <- inst
		}
	}
	close(results)
}

func (s *Session) query_region(f Filter, region string) <-chan *ec2.Instance {
	results := make(chan *ec2.Instance)
	client := ec2.New(s.s, &aws.Config{Region: aws.String(region)})
	describe := &ec2.DescribeInstancesInput{Filters: f.rules}
	output, err := client.DescribeInstances(describe)
	die(err)
	go collect(output, results)
	return results
}

func (s *Session) query(f Filter, regions []string) <-chan *ec2.Instance {
	responses := []<-chan *ec2.Instance{}
	for _, r := range regions {
		response := s.query_region(f, r)
		responses = append(responses, response)
	}
	return merge(responses)
}

type Filter struct {
	rules []*ec2.Filter
}

func (f Filter) String() string {
	var repr string
	var rules []string
	for _, rule := range f.rules {
		repr = rule.String()
		rules = append(rules, repr)
	}
	return strings.Join(rules, ",")
}

func (f *Filter) add(name string, vals ...string) {
	criteria := []*string{}
	for _, val := range vals {
		criteria = append(criteria, aws.String(val))
	}
	rule := &ec2.Filter{Name: &name, Values: criteria}
	f.rules = append(f.rules, rule)
}

func sanitised(args map[string]interface{}) map[string]string {
	m := make(map[string]string)
	for option, value := range args {
		if value != nil && value != false {
			k := tag(undash(option)) // --env -> tag:Env
			v := value.(string)
			m[k] = v
		}
	}
	return m
}

func tag(name string) string {
	const spec string = "tag:%s"
	return fmt.Sprintf(spec, name)
}

func undash(s string) string {
	no_dashes := strings.Replace(s, "-", "", -1)
	return strings.Title(no_dashes) // --env -> Env
}

func die(err error) {
	if err != nil {
		log.Fatalf("fatal: %s", err)
	}
}

func summarise(inst *ec2.Instance) {
	//fmt.Println(inst)
}

func merge(cs []<-chan *ec2.Instance) <-chan *ec2.Instance {
	var wg sync.WaitGroup
	results := make(chan *ec2.Instance)
	output := func(c <-chan *ec2.Instance) {
		for n := range c {
			results <- n
		}
		wg.Done()
	}
	wg.Add(len(cs))
	for _, c := range cs {
		go output(c)
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	return results
}

func main() {

	usage := `AWS hosts
    Usage:
      hosts [-r role] [-c customer] [-e environment]
      hosts --version

      Options:
      -h --help               Show this screen.
      -r, --role role         find hosts in these roles
      -c, --customer customer find hosts related to this customer
      -e, --env env           find hosts in this environment
   `
	var filter Filter
	session := NewSession()
	args, _ := docopt.Parse(usage, nil, true, VERSION, false)
	regions := []string{"us-east-1", "eu-west-1"}
	filter.add("instance-state-name", "running", "pending")
	for option, value := range sanitised(args) {
		filter.add(option, value)
	}
	instances := session.query(filter, regions)
	for inst := range instances {
		summarise(inst)
	}
}
