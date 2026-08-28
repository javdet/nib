## The web interface allows you to
Analyze data from various sources
Use analytics to plan task execution flows
Perform all necessary preparations for change implementation
Change implementation and subsequent success analysis

## A web application is planned in which
MCP connectors can be defined for all necessary systems
Direct connectors to certain systems, such as GitHub, Slack, etc.
Manage skills related to a specific area, such as working with cloud resources, working with metrics, etc.
Manage complex skills, which in turn call upon the skills described above
Ability to connect different LLMs
Use a knowledge base during workflow. Data loading can be performed both manually and via AI
Work with IaC using rules and skills
Manage CI/CD processes
Announcements of changes, launching deployments, and verification

## One of the main workflows
A task is entered.
Additional information is required to solve the task. Cloud infrastructure data
IaC code data
Metrics, possibly logs
And so on
AI creates a change plan (and rollback) based on the infrastructure specifics and the best solution to the problem
AI executes the plan, creating a PR, building all the necessary components for the release, and running tests
AI announces the changes and begins the deployment process
Then check the metrics and logs to see if everything is in order.
If not, it applies a rollback or recompiles the rules and plans fixes
Closes tasks, notifies stakeholders

Work can be distributed across multiple workers
The backend can also perform everything just like a worker

The main value: freeing the infrastructure manager from the need to independently collect information from various sources, correlate it, plan work, prepare, carry out work, and verify.

This won't completely replace everything, but that's the point.
To begin with, let's focus on the analytical part and data preparation.

When working, we will need to take into account such entities as the project, location, environment,