import React from "react";
import { Navbar, Container, Nav } from "react-bootstrap";

export default function Header() {
  return (
    <Navbar expand="lg" className='navbar_wrapper theme-navbar'>
      <Container>
        <Navbar.Brand href="#home"> <img src={require('../images/logo.png')} className="img-fluid" alt="logo" /> </Navbar.Brand>
        <Navbar.Toggle aria-controls="basic-navbar-nav" />
        <Navbar.Collapse id="basic-navbar-nav" className="flex--md-fill justify-content-md-end">
          <Nav className='navbar_nav me-auto'>
            <Nav.Link className='navbar_link' href="/">Home</Nav.Link>
            <Nav.Link className='navbar_link' href="/projects">Project</Nav.Link>
            <Nav.Link className='navbar_link' href="https://docs.muse.dev/docs/bugbash/">Documentation</Nav.Link>
            <Nav.Link className='navbar_link' href="https://lift.sonatype.com/">Install Lift</Nav.Link>
          </Nav>
        </Navbar.Collapse>
      </Container>
    </Navbar>
  );
}
