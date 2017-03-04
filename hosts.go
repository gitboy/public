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
)

const VERSION string = "hosts-1.0"

func first_tag_matching(key string, inst *ec2.Instance) string {
	for _, tag := range inst.Tags {
		if *tag.Key == key {
			return fmt.Sprintf("%s=%s", strings.ToLower(key), *tag.Value)
		}
	}
	return ""
}

func build_filter(args map[string]interface{}) []*ec2.Filter {
	// map args keys --customer -> Customer and return a tag filter
	filters := []*ec2.Filter{}
	for option, value := range args {
		if value != nil && value != false {
			name := strings.Title(option[2:]) // --customer -> Customer
			filter_name := fmt.Sprintf("tag:%s", name)
			filters = append(filters, &ec2.Filter{
				Name:   aws.String(filter_name),
				Values: []*string{aws.String(value.(string))},
			})
		}
	}
	running := &ec2.Filter{
		Name:   aws.String("instance-state-name"),
		Values: []*string{aws.String("running"), aws.String("pending")},
	}
	filters = append(filters, running) // limit to running or pending instances
	return filters
}

func Instances(region string, sess *session.Session, filter []*ec2.Filter) <-chan *ec2.Instance {
	// first stab at pulling pending/running instances from an AWS region
	out := make(chan *ec2.Instance, 10)
	client := ec2.New(sess, &aws.Config{Region: aws.String(region)})
	params := &ec2.DescribeInstancesInput{Filters: filter}
	resp, err := client.DescribeInstances(params)
	die(err)
  go func() {
    for idx, _ := range resp.Reservations {
      for _, inst := range resp.Reservations[idx].Instances {
        out <- inst
      }
    }
    close(out)
  }()
	return out
}

func die(err error) {
	if err != nil {
		log.Fatalf("fatal: %s", err)
	}
}

func summarise(inst *ec2.Instance) {
	var name, env, customer string
	name = first_tag_matching("Name", inst)
	env = first_tag_matching("Env", inst)
	customer = first_tag_matching("Customer", inst)
	if name == "" {
		log.Printf("warning: %s in %s is missing a name tag",
			*inst.InstanceId,
			*inst.Placement.AvailabilityZone)
	} else {
		fmt.Println(name, env, customer,
			first_tag_matching("Role", inst),
			*inst.InstanceId,
			*inst.Placement.AvailabilityZone,
			*inst.PrivateIpAddress)
	}
}

func merge(cs []<-chan *ec2.Instance) <-chan *ec2.Instance {
  // shamelessly lifted and modifiedfrom interwebz
	var wg sync.WaitGroup
	out := make(chan *ec2.Instance)
	output := func(c <-chan *ec2.Instance) {
		for n := range c {
			out <- n
		}
		wg.Done()
	}
	wg.Add(len(cs))
	for _, c := range cs {
	  go output(c)
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
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

	args, _ := docopt.Parse(usage, nil, true, VERSION, false)
	regions := []string{"us-east-1", "eu-west-1"}
	instances := make([]<- chan *ec2.Instance, len(regions))
	filter := build_filter(args)
	sess, err := session.NewSession()
	die(err)
	for i, region := range regions {
		instances[i] = Instances(region, sess, filter)
	}
	for inst := range merge(instances) {
		summarise(inst)
	}
  fmt.Println("==============")
}
