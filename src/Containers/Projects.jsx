import React, { useState } from "react";
import { Container, Tabs, Tab } from "react-bootstrap";
import Parser from "html-react-parser";
import Projectslogo from "../Components/Projectslogo";
import Footer from "../Components/Footer";

export default function Projects() {
  const [projects, setProjects] = useState([
    {
      id: "0",
      name: "Harbor",
      data: [
        {
          image: require("../images/tab-1.png"),
          description:
            "Harbor is an open source trusted cloud native registry project that stores, signs, and scans content.",
          text: "",
        },
      ],
    },
    {
      id: "1",
      name: "Kyverno",
      data: [
        {
          image: require("../images/tab-2.png"),
          description: `<a href="https://kyverno.io/">Kyverno</a> is a policy engine designed for Kubernetes. It can validate, mutate, and generate configurations using admission controls and background scans. Kyverno policies are Kubernetes resources and do not require learning a new language. Kyverno is designed to work nicely with tools you already use like kubectl, kustomize, and Git.`,
          text: "",
        },
      ],
    },
    {
      id: "2",
      name: "haos Mesh",
      data: [
        {
          image: require("../images/tab-3.png"),
          description: `<a href="https://chaos-mesh.org/">Chaos Mesh®</a>  is a Cloud Native Computing Foundation (CNCF) hosted project. It is a cloud-native Chaos Engineering platform that orchestrates chaos in Kubernetes environments. It originated from the testing framework of TiDB, a distributed database, so it takes into account the possible faults of a distributed system. Chaos Mesh can perform all around fault injections into the network, system clock, JVM applications, filesystems, operating systems, and so on. Chaos experiments are defined in YAML, which is fast and easy to use.`,
          text: "Maintainers",
          links:
            "Keao Yang, yangkeao@pingcap.com<br/>Zhiqiang Zhou, zhouzhiqiang@pingcap.com<br/>Xiang Wang, wangxiang@pingcap.com",
        },
      ],
    },
    {
      id: "3",
      name: "Keptn",
      data: [
        {
          image: require("../images/tab-4.png"),
          description: `<a href="https://keptn.sh/">Keptn</a> is an event-based control plane for continuous delivery and automated operations for cloud-native applications.`,
          text: "",
        },
      ],
    },
    {
      id: "4",
      name: "TiKV",
      data: [
        {
          image: require("../images/tab-5.png"),
          description: `<a href="https://tikv.org/">TiKV</a> is an open-source, distributed, and transactional key-value database.`,
          text: "",
        },
      ],
    },
    {
      id: "5",
      name: "Longhorn",
      data: [
        {
          image: require("../images/tab-6.png"),
          description: `<a href="https://longhorn.io/">Longhorn</a> is a distributed block storage system for Kubernetes. Longhorn is cloud native storage because it is built using Kubernetes and container primitives.`,
          text: "",
        }
      ],
    },
    {
      id: "6",
      name: "KubeVela",
      data: [
        {
          image: require("../images/tab-7.png"),
          description: `<a href="https://kubevela.io/">KubeVela</a> is a modern application platform that makes deploying and managing applications across today's hybrid, multi-cloud environments easier and faster.`,
          text: "",
        },
      ],
    },
    {
      id: "7",
      name: "Meshery",
      data: [
        {
          image: require("../images/tab-8.png"),
          description: `<a href="https://meshery.io/">Meshery</a> is the multi-service mesh management plane offering lifecycle, configuration, and performance management of service meshes and their workloads.`,
          text: "",
        },
      ],
    },
    {
      id: "8",
      name: "Serverless Workflow",
      data: [
        {
          image: require("../images/tab-9.png"),
          description: `<a href="https://serverlessworkflow.io/">Serverless</a> Workflow is a vendor-neutral, open-source and community-driven workflow ecosystem.`,
          text: "",
        },
      ],
    },
  ]);
  console.log("projects", projects);
  return (
    <div>
      <div className="banner-wrapper">
        <Container className="text-center">
          <img
            src={require("../images/project-banner-img.png")}
            className="img-fluid"
            alt="banner"
          />
        </Container>
      </div>
      <div className="projects_wrapper">
        <Container>
          <div className="project_section mb-5">
            <p className="w-75 mx-auto text-center">
              This is where you can keep track of the CNCF projects we're
              partnering with and their selected bug lists. We used Sonatype
              Lift to scan their repos and analyze for a list of bugs.
            </p>
            <p className="text-center mb-5">Select a bug and smash it!</p>
            <div className="tabs_wrapper">
              <Tabs
                defaultActiveKey="Harbor"
                id="uncontrolled-tab-example"
                className="mb-3"
              >
                {projects.map((project) => (
                  <Tab eventKey={project.name} title={project.name}>
                    <div className="tab_content px-3">
                      {project.data.map((d) => (
                        <>
                          <img
                            src={d.image}
                            className="img-fluid mx-auto"
                            alt="image"
                          />
                          <p className="mt-4">{Parser(d.description)}</p>
                          <ul className="list-inline">
                            <li>
                              <a href="https://github.com/goharbor/harbor/blob/master/CONTRIBUTING.md">
                                HOW TO CONTRIBUTE
                              </a>
                            </li>
                            {project.name != "Longhorn" && (
                              <>
                                <li>
                                  <a href="https://github.com/goharbor/harbor">
                                    GITHUB
                                  </a>
                                </li>
                                <li>
                                  <a href="https://lift.sonatype.com/result/bhamail/harbor/01FHG1B5AJ8MXCH0A05BMBNWWT">
                                    BUG LIST
                                  </a>
                                </li>
                              </>
                            )}
                          </ul>
                          {d.text && (
                            <div>
                              <strong>{d.text}</strong>
                              <br />
                              <p>{Parser(d.links)}</p>
                            </div>
                          )}
                        </>
                      ))}
                    </div>
                  </Tab>
                ))}
              </Tabs>
            </div>
          </div>
          <Projectslogo/>
        </Container>
      </div>
      <Footer/>
    </div>
  );
}
