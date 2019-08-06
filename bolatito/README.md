This library is to make life easier for all go user out there



## Installing

### *go get*

    $ go get -u github.com/iamtito/go/bolatito

## Example

### Simple Slack Message

```golang
import (
	"fmt"

	"github.com/iamtito/go/bolatito"
)



func main() {
	
	alertMessage = ":smile: and my message goes here."
	bolatito.SendSimpleMessageToSlack("channelID", alertMessage, "MessaegTitle", "SLACK_TOKEN"])
}
```



## Contributing

You are very much welcome to contribute to this project.  Fork and
make a Pull Request, or create an Issue if you see any problem.

## License

BSD 2 Clause license