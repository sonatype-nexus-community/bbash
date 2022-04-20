import React, { useState } from "react";
import { Container } from "react-bootstrap";
import CardSection from "../Components/CardSection";
import PointsDetails from "../Components/PointsDetails";
import Parser from "html-react-parser";
import Projectslogo from "../Components/Projectslogo";
import Footer from "../Components/Footer";

export default function Home() {
  return (
    <div className="home-wrapper">
      <div className="banner-wrapper">
        <Container className="text-center">
          <img
            src={require("../images/banner-img.png")}
            className="img-fluid"
            alt="banner"
          />
          <p className="banner-details mb-0">
            Presented by CNCF + Sonatype | MONDAY OCTOBER 11TH | 10:00 AM PST
          </p>
        </Container>
      </div>
      <PointsDetails />
      <div className="welcome_wrapper bg-grey pt-5 pb-2">
        <Container className="text-center">
          <h1 className="section_heading">
            Welcome to the CNCF + Sonatype Bug Bash
          </h1>
          <p className="section_details mx-auto mb-4">
            Compete with your fellow developers to smash bugs, earn points, and
            win prizes! <br /> Playing is easy! We’ve partnered with CNCF
            projects, scanned their repos, and pulled together a list of bugs
            for each.
          </p>
          <CardSection
            title={"How to Play"}
            title_details={Parser(
              `If you haven't yet, please sign up on the <a href="#">CNCF Bug Bash site<a/> or scan the QR code. We use this info to get you up on the leaderboard!`
            )}
            image={require("../images/QR-code.png")}
            section_content={
              <ul className="list-inline">
                <li>
                  Step 1 : Install Lift at <a href="#">lift.sonatype.com</a>
                </li>
                <li>
                  Step 2 : Pick a <a href="#">Project</a>
                </li>
                <li>
                  Step 3 : Review their guidelines (pinned to top of the slack
                  channel)
                </li>
                <li>Step 4 : Find a Bug in the Bug List</li>
                <li>Step 5 : Smash the Bug</li>
                <li></li>
              </ul>
            }
          />
          <CardSection
            title={"How to Win"}
            title_details={
              "There are several winner categories in the Bash. You can keep an eye on the leaderboard to see where you stack up!"
            }
            image={require("../images/winner.png")}
            section_content={
              <>
                <strong>Most Points Overall</strong>
                <p>
                  You’ll earn a point for each bug you smash.The top ten bug
                  bashers are ranked live on the leaderboard and click see more
                  to see all participants.
                </p>

                <strong>Special Quests</strong>
                <p>
                  Some bugs are harder than others, and some bugs have a bigger
                  impact than others. Under several projects you will see their
                  quests for PRs that will be the most helpful contributions.
                </p>

                <strong>What You Win</strong>
                <p>
                  op 3 Contributors to the bug bash will win swag coupons for
                  the CNCF Store (1st - $100; 2nd - $75; 3rd - $50) + 100%
                  discounts for a course from Linux Foundation.
                  <br />
                  Everyone who submits even one Bug will get a 65% discount on
                  the Linux Foundation Training of their choice.
                </p>
                <em>
                  All bugs submitted valid between 10am Monday Oct 11th and 5pm
                  Thursday Oct 14th PST.
                </em>
              </>
            }
          />
          <CardSection
            title={"How to Smash a Bug"}
            title_details={""}
            image={require("../images/bug.png")}
            section_content={
              <>
                <strong>Fork Chosen Project and Clone</strong>
                <br />
                <br />
                <strong>Smash Bug in Local Environment</strong>
                <p>
                  ‍Usually best done with a keyboard, but sometimes a hammer is
                  the only thing to get the job done
                </p>
                <strong>
                  IMPORTANT! Create a Pull Request back to Upstream GitHub
                  Repository
                </strong>
                <p>
                  ‍This is how you get your points, so don’t forget it! If you
                  have any questions throughout the bash, reach out on the
                  #4-kubecon-bugbash channel. Our hosts and maintainers will be
                  in and out and happy to help.
                </p>
              </>
            }
          />
          <Projectslogo />
        </Container>
      </div>
      <Footer />
    </div>
  );
}
