//
// @bp0lr - 30/11/2020
//

package main

import (
	"io"
	"os"
	"fmt"
	"sync"
	"net/http"
	"time"
	"bufio"
	"io/ioutil"
	"strings"
	"strconv"
	"net/url"
	"math/rand"
		
	resolver	"github.com/bp0lr/dmut/resolver"
	flag 		"github.com/spf13/pflag"
	tld 		"github.com/weppos/publicsuffix-go/publicsuffix"
)

var (
	workersArg	      	int
	dnsTimeOutArg		int
	mutationsDic		string
	urlArg            	string
	outputFileArg     	string
	dnsFileArg			string
	dnsArg				string
	verboseArg        	bool
	updateDNSArg		bool
	ipArg				bool
	simulateArg			bool
	dnsServers 			[]string	
)

//jobL desc
type jobL struct {
	tld        	string
	sld  		string
	trd  		string
	tasks		[]string
}

func main() {

	var alterations []string

	flag.IntVarP(&workersArg, "workers", "w", 25, "Workers amount")
	flag.StringVarP(&urlArg, "url", "u", "", "Single url to test")
	flag.BoolVarP(&verboseArg, "verbose", "v", false, "Display extra info about what is going on")
	flag.StringVarP(&mutationsDic, "dic", "d", "", "Dictionary file containing mutation list")
	flag.StringVarP(&outputFileArg, "output", "o", "", "Output file to save the results to")
	flag.StringVarP(&dnsFileArg, "dnsFile", "s", "", "Use this dns server list from file")
	flag.StringVarP(&dnsArg, "dnsServers", "l", "", "Use this dns server list separated by ,")
	flag.BoolVar(&updateDNSArg, "update-dnslist", false, "Use this dns server list separated by ,")
	flag.BoolVar(&ipArg, "show-ip", false, "Display info for valid results")
	flag.BoolVar(&simulateArg, "simulate", false, "Display info about the job without run it")
	flag.IntVar(&dnsTimeOutArg, "dns-timeout", 500, "Dns Server timeOut in millisecond")

	flag.Parse()


	if(updateDNSArg){
		err:=downloadResolverList()
		if(err != nil){
			fmt.Printf("[-] Error: %v\n", err)
		}else{
			fmt.Printf("[+] File downloaded successfully\n")
		}
		return
	}

	//concurrency
	workers := 25
	if workersArg > 0  && workersArg < 100 {
		workers = workersArg
	}
	
	if(dnsTimeOutArg == 0 || dnsTimeOutArg > 10000) {
		dnsTimeOutArg = 500
	}

	if(verboseArg){
		fmt.Printf("[+] Workers: %v\n", workers)
	}

	if(len(dnsFileArg) > 0){
		if _, err := os.Stat(dnsFileArg); os.IsNotExist(err) {
			fmt.Printf("Error, %v\n", err)
			return
		  }else{
			content, _ := ioutil.ReadFile(dnsFileArg)
			dnsServers = strings.Split(string(content), "\n")
		  }
	}

	if(len(mutationsDic) == 0){
		fmt.Printf("Error, you need to define a mutation file list using the arg -d\n")
		return
	}

	if(len(mutationsDic) > 0){
		if _, err := os.Stat(mutationsDic); os.IsNotExist(err) {
			fmt.Printf("Error, %v\n", err)
			return
		}else{
			mutations, _ := ioutil.ReadFile(mutationsDic)
			alterations = strings.Split(string(mutations), "\n")
		}
	}

	if(len(dnsArg) > 0){
		dnsServers = strings.Split(dnsArg, ",")
	}

	if(len(dnsServers) > 0 ){
		if(verboseArg){
			fmt.Printf("[+] we are using this dns servers: %v\n", dnsServers)
			fmt.Printf("[+] Current dns timeout is: %v\n", dnsTimeOutArg)
		}
	}

	var outputFile *os.File
	var err0 error
	if outputFileArg != "" {
		outputFile, err0 = os.OpenFile(outputFileArg, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
		if err0 != nil {
			fmt.Printf("cannot write %s: %s", outputFileArg, err0.Error())
			return
		}
		
		defer outputFile.Close()
	}

	var jobs []string

	if len(urlArg) < 1 {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			jobs = append(jobs, sc.Text())
		}
	} else {
		jobs = append(jobs, urlArg)
	}

	for _, value := range jobs {
		processDomain(workers, value, alterations, outputFile, dnsTimeOutArg)
	}

	//if we didn't found anything, delete the result file.
	if(len(outputFileArg) > 0){
		fi, err := os.Stat(outputFileArg)
		if err == nil {
			if(fi.Size() == 0){
				os.Remove(outputFileArg)
			}
		}
	}	
}

func processDomain(workers int, domain string, alterations [] string, outputFile *os.File, dnsTimeout int){

	_, err := url.Parse(domain)
	if err != nil {
		if verboseArg {
			fmt.Printf("[-] Invalid url: %s\n", domain)
		}
		return
	}		
	
	domParse, _:=tld.Parse(domain)
	
	//tld: com
	//sld: redbull
	//trd: lala.test.val

	var job jobL	
	job.sld = domParse.SLD
	job.tld = domParse.TLD
	job.trd = domParse.TRD
	
	//	this will add each alteration to the existing domain.
	//	for example to test some.test.com we are going to generate alt1.some.test.com and some.alt1.test.com
	///////////////////////////////////////////////////////////////////////////////////////////////			
	for _, alt := range alterations {

		if(len(alt) < 1){
			continue
		}

		strSplit := strings.Split(job.trd, ".")

		for i := 0; i <= len(strSplit); i++ {
			val:=insert(strSplit, i, alt)
			job.tasks = append(job.tasks, strings.Join(val, "."))
		}
	}
	
	//	this will add a number to the end of each subdmain part.
	//	for example to test some.test.com we are going to generate some1.some.test.com, some2.alt1.test.com, etc
	///////////////////////////////////////////////////////////////////////////////////////////////		
	for index := 0; index < 10; index++ {		
		strSplit := strings.Split(job.trd, ".")

		for i := 0; i < len(strSplit); i++ {
			
			strclean:=strSplit[i]
			strSplit[i] = strclean+"-"+strconv.Itoa(index)
			job.tasks = append(job.tasks, strings.Join(strSplit, "."))
			
			strSplit[i] = strclean+strconv.Itoa(index)
			job.tasks = append(job.tasks, strings.Join(strSplit, "."))
		}
	}
	
	//	this will add (clean and using a -) each alteration to each subdomain part.
	//	for example to test some.test.com we are going to generate some-alt1.test.com, alt1-some.test.com, etc
	///////////////////////////////////////////////////////////////////////////////////////////////
	for _, alt := range alterations {	
		if(len(alt) < 1){
			continue
		}

		strSplit := strings.Split(job.trd, ".")

		for i := 0; i < len(strSplit); i++ {			
			strclean:=strSplit[i]

			strSplit[i] = strclean+"-"+alt
			job.tasks = append(job.tasks, strings.Join(strSplit, "."))

			strSplit[i] = alt+"-"+strclean
			job.tasks = append(job.tasks, strings.Join(strSplit, "."))

			strSplit[i] = strclean+alt
			job.tasks = append(job.tasks, strings.Join(strSplit, "."))

			strSplit[i] = alt+strclean
			job.tasks = append(job.tasks, strings.Join(strSplit, "."))
		}
	}
	
	//removing duplicated from job.tasks
	job.tasks = removeDuplicatesSlice(job.tasks)
	
	if(verboseArg){
		fmt.Printf("[%v] We have %v jobs to do.\n", domain, len(job.tasks))
	}

	if(simulateArg){
		return
	}
	
	jobs := make(chan string)
	var wg sync.WaitGroup
	
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			for task := range jobs {				
				fullDomain:= task + "." + job.sld + "." + job.tld + "."
				processDNS(&wg, fullDomain, outputFile, dnsTimeout)
			}
			wg.Done()
		}()
	}
		
	for _, line := range job.tasks {
		jobs <- line
	}
	
	close(jobs)	
	wg.Wait()	
}

func processDNS(wg *sync.WaitGroup, domain string, outputFile *os.File, dnsTimeout int) {

	trimDomain:= trimLastPoint(domain, ".")

	if verboseArg {
		fmt.Printf("[+] Testing: %v\n", domain)
	}

	qtypes := []string{"A", "CNAME", "AAAA"}

	var dnsServer string
	if(len(dnsServers) > 0){
		rand.Seed(time.Now().UnixNano())
		randomIndex := rand.Intn(len(dnsServers) -1)

		dnsServer = dnsServers[randomIndex] + ":53"
	}else{
		dnsServer = "8.8.8.8:53"
	}

	if(verboseArg){
		fmt.Printf("[+] Using dns server: %v\n", dnsServer)
	}

	for _, qtype := range qtypes {
		result, err:= resolver.GetDNSQueryResponse(qtype, domain, dnsServer, dnsTimeout)
		if err == nil  && len(result) > 0{
			if outputFileArg != "" {
				if(ipArg){				
					outputFile.WriteString(trimDomain + result + "\n")
				} else {
					outputFile.WriteString(trimDomain + "\n")
				}
			}	
			if(ipArg){
				fmt.Printf("[VALID] %v%v\n", trimDomain, result)
			}else{
				fmt.Printf("%v\n", trimDomain)
			}
			break
		} else{
			if(verboseArg){
				//fmt.Printf("err: %v\n", err)
			}
		}
	}	
}

func insert(a []string, index int, value string) []string {
    if len(a) == index { // nil or empty slice or after last element
        return append(a, value)
    }
    a = append(a[:index+1], a[index:]...) // index < len(a)
    a[index] = value
    return a
}

func downloadResolverList() error{
		
	out, err := os.Create("resolvers.txt")
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get("https://raw.githubusercontent.com/janmasarik/resolvers/master/resolvers.txt")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func trimLastPoint(s, suffix string) string {
    if strings.HasSuffix(s, suffix) {
        s = s[:len(s)-len(suffix)]
    }
    return s
}

func removeDuplicatesSlice(s []string) []string {
	m := make(map[string]bool)
	for _, item := range s {
			if _, ok := m[item]; ok {
					//fmt.Printf("Duplicate: %v\n", item)
			} else {
					m[item] = true
			}
	}

	var result []string
	for item := range m {
		result = append(result, item)
	}

	return result
}